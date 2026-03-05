package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

// CopilotDeviceSession tracks a pending device OAuth flow.
type CopilotDeviceSession struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"`
	NextPollAt      time.Time `json:"next_poll_at"` // earliest time for next poll
}

// CopilotOAuthResult is the result of a successful device OAuth flow.
type CopilotOAuthResult struct {
	GitHubLogin string `json:"github_login"`
	GitHubName  string `json:"github_name"`
	GitHubID    int64  `json:"github_id"`
}

// CopilotOAuthService handles the GitHub device OAuth flow for Copilot accounts.
type CopilotOAuthService struct {
	httpClient *http.Client

	mu       sync.Mutex
	sessions map[string]*CopilotDeviceSession // sessionID → session
}

// NewCopilotOAuthService creates a new CopilotOAuthService.
func NewCopilotOAuthService() *CopilotOAuthService {
	return &CopilotOAuthService{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		sessions:   make(map[string]*CopilotDeviceSession),
	}
}

// StartDeviceFlow initiates the GitHub device OAuth flow.
// Returns the device code response including the user_code and verification_uri.
func (s *CopilotOAuthService) StartDeviceFlow(ctx context.Context) (*CopilotDeviceSession, string, error) {
	resp, err := copilot.RequestDeviceCode(s.httpClient)
	if err != nil {
		return nil, "", fmt.Errorf("start device flow: %w", err)
	}

	// Generate a session ID to track this flow
	sessionID := fmt.Sprintf("copilot_%d", time.Now().UnixNano())

	session := &CopilotDeviceSession{
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresAt:       time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		Interval:        resp.Interval,
		NextPollAt:      time.Now(), // can poll immediately
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	// Clean up expired sessions in the background
	go s.cleanupExpiredSessions()

	slog.Info("copilot device flow started",
		"session_id", sessionID,
		"user_code", resp.UserCode,
		"verification_uri", resp.VerificationURI)

	return session, sessionID, nil
}

// PollDeviceFlow polls GitHub for the access token.
// Returns the access token and user info on success, or an error describing the current state.
func (s *CopilotOAuthService) PollDeviceFlow(ctx context.Context, sessionID string) (string, *CopilotOAuthResult, error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	s.mu.Unlock()

	if !ok {
		return "", nil, fmt.Errorf("session not found or expired: %s", sessionID)
	}

	if time.Now().After(session.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return "", nil, fmt.Errorf("device code expired")
	}

	// Enforce GitHub's required polling interval.
	// If the frontend polls faster than allowed, return pending without hitting GitHub.
	s.mu.Lock()
	nextPoll := session.NextPollAt
	s.mu.Unlock()
	if time.Now().Before(nextPoll) {
		remaining := time.Until(nextPoll).Round(time.Second)
		slog.Debug("poll too fast, enforcing interval",
			"session_id", sessionID,
			"wait", remaining)
		return "", nil, fmt.Errorf("authorization_pending")
	}

	// Poll GitHub for the access token
	tokenResp, err := copilot.PollAccessToken(s.httpClient, session.DeviceCode)
	if err != nil {
		return "", nil, fmt.Errorf("poll access token: %w", err)
	}

	// Check for pending/slow_down states
	if tokenResp.Error != "" {
		if tokenResp.Error == "authorization_pending" {
			// Update next poll time based on current interval
			s.mu.Lock()
			session.NextPollAt = time.Now().Add(time.Duration(session.Interval) * time.Second)
			s.mu.Unlock()
			return "", nil, fmt.Errorf("authorization_pending")
		}
		if tokenResp.Error == "slow_down" {
			// GitHub requires increasing the interval by 5s; honor the returned interval
			newInterval := session.Interval + 5
			if tokenResp.Interval > 0 {
				newInterval = tokenResp.Interval
			}
			s.mu.Lock()
			session.Interval = newInterval
			session.NextPollAt = time.Now().Add(time.Duration(newInterval) * time.Second)
			s.mu.Unlock()
			slog.Warn("copilot poll slow_down, backing off",
				"session_id", sessionID,
				"new_interval_sec", newInterval)
			return "", nil, fmt.Errorf("authorization_pending")
		}
		if tokenResp.Error == "expired_token" {
			s.mu.Lock()
			delete(s.sessions, sessionID)
			s.mu.Unlock()
			return "", nil, fmt.Errorf("device code expired")
		}
		return "", nil, fmt.Errorf("oauth error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return "", nil, fmt.Errorf("empty access token in response")
	}

	// Success! Clean up the session
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	// Verify token works by fetching user info
	user, err := copilot.GetGitHubUser(s.httpClient, tokenResp.AccessToken)
	if err != nil {
		slog.Warn("failed to fetch github user after device oauth", "error", err)
		// Token is valid even if user fetch fails
		return tokenResp.AccessToken, &CopilotOAuthResult{}, nil
	}

	// Verify the token can actually be used for Copilot (exchange test)
	_, exchangeErr := copilot.ExchangeToken(s.httpClient, tokenResp.AccessToken)
	if exchangeErr != nil {
		slog.Error("copilot exchange token failed after device oauth",
			"error", exchangeErr,
			"github_login", user.Login,
		)
		return "", nil, fmt.Errorf("token obtained but Copilot access not available: %w (user: %s)", exchangeErr, user.Login)
	}

	slog.Info("copilot device oauth completed",
		"github_login", user.Login,
		"github_id", user.ID)

	result := &CopilotOAuthResult{
		GitHubLogin: user.Login,
		GitHubName:  user.Name,
		GitHubID:    user.ID,
	}

	return tokenResp.AccessToken, result, nil
}

func (s *CopilotOAuthService) cleanupExpiredSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}
