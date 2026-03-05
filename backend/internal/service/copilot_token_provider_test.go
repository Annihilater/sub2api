package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

func TestCopilotTokenProvider_GetAccessToken(t *testing.T) {
	provider := NewCopilotTokenProvider()

	t.Run("nil account", func(t *testing.T) {
		_, err := provider.GetAccessToken(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil account")
		}
	})

	t.Run("wrong platform", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
		}
		_, err := provider.GetAccessToken(context.Background(), account)
		if err == nil {
			t.Fatal("expected error for non-copilot account")
		}
	})

	t.Run("missing github_token", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformCopilot,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{},
		}
		_, err := provider.GetAccessToken(context.Background(), account)
		if err == nil {
			t.Fatal("expected error for missing github_token")
		}
	})

	t.Run("returns cached token", func(t *testing.T) {
		provider := NewCopilotTokenProvider()
		provider.tokens[42] = &copilot.CopilotToken{
			Token:     "cached-copilot-token",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			RefreshAt: time.Now().Add(5 * time.Minute),
		}

		account := &Account{
			ID:       42,
			Platform: PlatformCopilot,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"github_token": "ghp_valid",
			},
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "cached-copilot-token" {
			t.Errorf("token = %q, want %q", token, "cached-copilot-token")
		}
	})
}

func TestCopilotTokenProvider_InvalidateToken(t *testing.T) {
	provider := NewCopilotTokenProvider()
	provider.tokens[42] = &copilot.CopilotToken{
		Token:     "old-token",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		RefreshAt: time.Now().Add(5 * time.Minute),
	}

	provider.InvalidateToken(42)

	provider.mu.RLock()
	_, exists := provider.tokens[42]
	provider.mu.RUnlock()

	if exists {
		t.Error("token should have been invalidated")
	}
}
