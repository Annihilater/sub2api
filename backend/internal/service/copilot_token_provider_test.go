package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

// mockCopilotTokenServer creates a test server that returns a valid Copilot token.
func mockCopilotTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "token ghp_valid" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"copilot-test-token","expires_at":` +
			`2000000000,"refresh_in":1500}`))
	}))
}

func TestCopilotTokenProvider_GetAccessToken(t *testing.T) {
	provider := NewCopilotTokenProvider(nil)

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
		provider := NewCopilotTokenProvider(nil)
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
	provider := NewCopilotTokenProvider(nil)
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
