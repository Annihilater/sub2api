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

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

// CopilotTokenProvider manages Copilot API tokens for GitHub Copilot accounts.
//
// GitHub Copilot uses a two-step token flow:
//  1. A long-lived GitHub personal access token (stored in account credentials)
//  2. A short-lived Copilot API token (~30min) obtained via token exchange
//
// This provider handles the exchange and caching of Copilot API tokens.
type CopilotTokenProvider struct {
	accountRepo AccountRepository
	httpClient  *http.Client

	mu     sync.RWMutex
	tokens map[int64]*copilot.CopilotToken // accountID → cached token
}

// NewCopilotTokenProvider creates a new CopilotTokenProvider.
func NewCopilotTokenProvider(accountRepo AccountRepository) *CopilotTokenProvider {
	return &CopilotTokenProvider{
		accountRepo: accountRepo,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		tokens:      make(map[int64]*copilot.CopilotToken),
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

	// Check cached token
	p.mu.RLock()
	cached, hasCached := p.tokens[account.ID]
	p.mu.RUnlock()

	if hasCached && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
		return cached.Token, nil
	}

	// Need to refresh — acquire write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	cached, hasCached = p.tokens[account.ID]
	if hasCached && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
		return cached.Token, nil
	}

	// If token exists and is still valid (just needs refresh), return it while refreshing
	// This avoids blocking requests during refresh.
	var fallbackToken string
	if hasCached && cached != nil && !cached.IsExpired() {
		fallbackToken = cached.Token
	}

	// Exchange GitHub token for Copilot token
	newToken, err := copilot.ExchangeToken(p.httpClient, githubToken)
	if err != nil {
		slog.Error("copilot token exchange failed",
			"account_id", account.ID,
			"error", err)

		// Return the old token if still valid
		if fallbackToken != "" {
			return fallbackToken, nil
		}
		return "", fmt.Errorf("copilot token exchange: %w", err)
	}

	p.tokens[account.ID] = newToken
	slog.Debug("copilot token refreshed",
		"account_id", account.ID,
		"expires_at", newToken.ExpiresAt.Format(time.RFC3339))

	return newToken.Token, nil
}

// InvalidateToken removes the cached token for the given account.
func (p *CopilotTokenProvider) InvalidateToken(accountID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, accountID)
}
