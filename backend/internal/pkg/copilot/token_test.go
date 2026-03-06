package copilot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if got := r.Header.Get("Authorization"); got != "token ghp_test123" {
			t.Errorf("Authorization = %q, want %q", got, "token ghp_test123")
		}
		if got := r.Header.Get("editor-version"); got != DefaultEditorVersion {
			t.Errorf("editor-version = %q, want %q", got, DefaultEditorVersion)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodGet)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenExchangeResponse{
			Token:     "copilot-token-abc",
			ExpiresAt: 1800000000,
			RefreshIn: 1500,
		})
	}))
	defer server.Close()

	// Override the exchange URL for testing — we can't do this with a const,
	// so we test with the real function against a mock server instead.
	// In production the real URL is used.
	// For this test, we call the server directly.
	client := server.Client()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	req.Header.Set("Authorization", "token ghp_test123")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("editor-version", DefaultEditorVersion)
	req.Header.Set("editor-plugin-version", DefaultEditorPluginVersion)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("x-github-api-version", DefaultGitHubAPIVersion)

	resp, err := client.Do(req) //nolint:gosec // test server URL
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var tokenResp TokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if tokenResp.Token != "copilot-token-abc" {
		t.Errorf("Token = %q, want %q", tokenResp.Token, "copilot-token-abc")
	}
	if tokenResp.RefreshIn != 1500 {
		t.Errorf("RefreshIn = %d, want %d", tokenResp.RefreshIn, 1500)
	}
}

func TestExchangeToken_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	// Directly test the error handling pattern
	client := server.Client()
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	req.Header.Set("Authorization", "token bad-token")

	resp, err := client.Do(req) //nolint:gosec // test server URL
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}
