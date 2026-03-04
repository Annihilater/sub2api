package copilot

import (
	"testing"
	"time"
)

func TestCopilotToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "expired token",
			expiresAt: time.Now().Add(-10 * time.Minute),
			want:      true,
		},
		{
			name:      "within safety margin (30s left)",
			expiresAt: time.Now().Add(30 * time.Second),
			want:      true,
		},
		{
			name:      "valid token",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &CopilotToken{
				Token:     "test-token",
				ExpiresAt: tt.expiresAt,
				RefreshAt: time.Now().Add(-1 * time.Minute),
			}
			if got := token.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopilotToken_ShouldRefresh(t *testing.T) {
	tests := []struct {
		name      string
		refreshAt time.Time
		want      bool
	}{
		{
			name:      "past refresh time",
			refreshAt: time.Now().Add(-1 * time.Minute),
			want:      true,
		},
		{
			name:      "future refresh time",
			refreshAt: time.Now().Add(10 * time.Minute),
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &CopilotToken{
				Token:     "test-token",
				ExpiresAt: time.Now().Add(30 * time.Minute),
				RefreshAt: tt.refreshAt,
			}
			if got := token.ShouldRefresh(); got != tt.want {
				t.Errorf("ShouldRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopilotHeaders(t *testing.T) {
	t.Run("default initiator", func(t *testing.T) {
		h := CopilotHeaders("", false)
		if got := h.Get("X-Initiator"); got != "user" {
			t.Errorf("X-Initiator = %q, want %q", got, "user")
		}
		if got := h.Get("editor-version"); got != DefaultEditorVersion {
			t.Errorf("editor-version = %q, want %q", got, DefaultEditorVersion)
		}
		if got := h.Get("Copilot-Vision-Request"); got != "" {
			t.Errorf("Copilot-Vision-Request should be empty, got %q", got)
		}
	})

	t.Run("agent initiator with vision", func(t *testing.T) {
		h := CopilotHeaders("agent", true)
		if got := h.Get("X-Initiator"); got != "agent" {
			t.Errorf("X-Initiator = %q, want %q", got, "agent")
		}
		if got := h.Get("Copilot-Vision-Request"); got != "true" {
			t.Errorf("Copilot-Vision-Request = %q, want %q", got, "true")
		}
	})
}
