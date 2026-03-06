package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

// CopilotTokenProvider manages Copilot API tokens for GitHub Copilot accounts.
//
// GitHub Copilot uses a two-step token flow:
//  1. A long-lived GitHub personal access token (stored in account credentials)
//  2. A short-lived Copilot API token (~30min) obtained via token exchange
//
// This provider handles the exchange and caching of Copilot API tokens.
//
// Concurrency design:
//   - A read lock guards cache lookups so multiple goroutines can read concurrently.
//   - A singleflight group ensures that only ONE exchange HTTP call is in-flight
//     per account at a time.  Concurrent goroutines that need a fresh token for
//     the same account will block on the single in-flight call and all receive
//     the same result — no thundering-herd on the GitHub token exchange endpoint.
//   - The global write lock is held only for the brief cache-write after the
//     exchange returns, NOT during the HTTP I/O itself.
type CopilotTokenProvider struct {
	httpClient *http.Client

	mu     sync.RWMutex
	tokens map[int64]*copilot.CopilotToken // accountID → cached token

	// sfGroup deduplicates concurrent token-exchange requests for the same account.
	sfGroup singleflight.Group
}

// NewCopilotTokenProvider creates a new CopilotTokenProvider.
func NewCopilotTokenProvider() *CopilotTokenProvider {
	return &CopilotTokenProvider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     make(map[int64]*copilot.CopilotToken),
	}
}

// GetAccessToken returns a valid Copilot API token for the given account.
//
// For API Key type accounts, the github_token credential is used to exchange
// for a short-lived Copilot token. Tokens are cached in memory and refreshed
// automatically when they approach expiry.
func (p *CopilotTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformCopilot {
		return "", errors.New("not a copilot account")
	}

	githubToken := strings.TrimSpace(account.GetCredential("github_token"))
	if githubToken == "" {
		return "", errors.New("copilot account missing github_token in credentials")
	}

	// Fast path: check cached token under read lock (no I/O).
	p.mu.RLock()
	cached, hasCached := p.tokens[account.ID]
	p.mu.RUnlock()

	if hasCached && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
		return cached.Token, nil
	}

	// Slow path: token is missing, expired, or approaching expiry.
	// Use singleflight to ensure only one exchange call per account is in-flight
	// at a time. Concurrent goroutines waiting on the same account share the
	// result without each making an independent HTTP request.
	key := fmt.Sprintf("exchange:%d", account.ID)
	val, err, _ := p.sfGroup.Do(key, func() (any, error) {
		// Re-check under read lock: another goroutine may have refreshed between
		// our first check and entering the singleflight group.
		p.mu.RLock()
		cached, hasCached := p.tokens[account.ID]
		p.mu.RUnlock()
		if hasCached && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
			return cached.Token, nil
		}

		// Capture fallback before exchanging (token still valid but near expiry).
		var fallbackToken string
		if hasCached && cached != nil && !cached.IsExpired() {
			fallbackToken = cached.Token
		}

		// Exchange GitHub token for Copilot token — no lock held during I/O.
		newToken, err := copilot.ExchangeToken(p.httpClient, githubToken)
		if err != nil {
			slog.Error("copilot token exchange failed",
				"account_id", account.ID,
				"error", err)
			if fallbackToken != "" {
				return fallbackToken, nil
			}
			return "", fmt.Errorf("copilot token exchange: %w", err)
		}

		// Store the new token under write lock (brief critical section, no I/O).
		p.mu.Lock()
		p.tokens[account.ID] = newToken
		p.mu.Unlock()

		slog.Debug("copilot token refreshed",
			"account_id", account.ID,
			"expires_at", newToken.ExpiresAt.Format(time.RFC3339))

		return newToken.Token, nil
	})
	if err != nil {
		return "", err
	}

	token, ok := val.(string)
	if !ok {
		return "", errors.New("copilot token provider: unexpected result type")
	}
	return token, nil
}

// InvalidateToken removes the cached token for the given account.
func (p *CopilotTokenProvider) InvalidateToken(accountID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, accountID)
}
