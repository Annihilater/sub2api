package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
	"github.com/gin-gonic/gin"
)

func TestDetectStreamMode(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"stream true", `{"model":"gpt-4","stream":true}`, true},
		{"stream false", `{"model":"gpt-4","stream":false}`, false},
		{"no stream field", `{"model":"gpt-4"}`, false},
		{"stream string", `{"model":"gpt-4","stream":"true"}`, false},
		{"stream null", `{"model":"gpt-4","stream":null}`, false},
		{"invalid json", `{invalid`, false},
		{"empty body", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectStreamMode([]byte(tt.body))
			if got != tt.want {
				t.Errorf("detectStreamMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopilotGatewayService_ApplyModelMapping(t *testing.T) {
	svc := &CopilotGatewayService{}

	t.Run("no mapping configured", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformCopilot,
			Credentials: map[string]any{},
		}
		body := []byte(`{"model":"gpt-4o","messages":[]}`)

		newBody, model := svc.applyModelMapping(body, account)
		if model != "gpt-4o" {
			t.Errorf("model = %q, want %q", model, "gpt-4o")
		}
		// Body should be unchanged
		var req map[string]json.RawMessage
		if err := json.Unmarshal(newBody, &req); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		var m string
		json.Unmarshal(req["model"], &m)
		if m != "gpt-4o" {
			t.Errorf("body model = %q, want %q", m, "gpt-4o")
		}
	})

	t.Run("with mapping", func(t *testing.T) {
		account := &Account{
			Platform: PlatformCopilot,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-4": "gpt-4o",
				},
			},
		}
		body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)

		newBody, model := svc.applyModelMapping(body, account)
		if model != "gpt-4" {
			t.Errorf("model = %q, want %q (original)", model, "gpt-4")
		}
		// Body should have mapped model
		var req map[string]json.RawMessage
		if err := json.Unmarshal(newBody, &req); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		var m string
		json.Unmarshal(req["model"], &m)
		if m != "gpt-4o" {
			t.Errorf("body model = %q, want %q", m, "gpt-4o")
		}
	})

	t.Run("empty model", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformCopilot,
			Credentials: map[string]any{},
		}
		body := []byte(`{"messages":[]}`)

		_, model := svc.applyModelMapping(body, account)
		if model != "" {
			t.Errorf("model = %q, want empty", model)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformCopilot,
			Credentials: map[string]any{},
		}
		body := []byte(`{invalid}`)

		retBody, model := svc.applyModelMapping(body, account)
		if model != "" {
			t.Errorf("model = %q, want empty", model)
		}
		if string(retBody) != string(body) {
			t.Errorf("body should be unchanged for invalid json")
		}
	})
}

func TestCopilotGatewayService_ParseStreamUsage(t *testing.T) {
	svc := &CopilotGatewayService{}

	t.Run("valid usage", func(t *testing.T) {
		usage := &CopilotUsage{}
		data := `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
		svc.parseStreamUsage(data, usage)

		if usage.PromptTokens != 10 {
			t.Errorf("PromptTokens = %d, want 10", usage.PromptTokens)
		}
		if usage.CompletionTokens != 20 {
			t.Errorf("CompletionTokens = %d, want 20", usage.CompletionTokens)
		}
		if usage.TotalTokens != 30 {
			t.Errorf("TotalTokens = %d, want 30", usage.TotalTokens)
		}
	})

	t.Run("no usage field", func(t *testing.T) {
		usage := &CopilotUsage{}
		data := `{"choices":[{"delta":{"content":"hi"}}]}`
		svc.parseStreamUsage(data, usage)

		if usage.TotalTokens != 0 {
			t.Errorf("TotalTokens = %d, want 0", usage.TotalTokens)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		usage := &CopilotUsage{}
		svc.parseStreamUsage("{invalid}", usage)

		if usage.TotalTokens != 0 {
			t.Errorf("TotalTokens = %d, want 0", usage.TotalTokens)
		}
	})

	t.Run("updates existing usage", func(t *testing.T) {
		usage := &CopilotUsage{PromptTokens: 5, CompletionTokens: 5, TotalTokens: 10}
		data := `{"usage":{"prompt_tokens":15,"completion_tokens":25,"total_tokens":40}}`
		svc.parseStreamUsage(data, usage)

		if usage.TotalTokens != 40 {
			t.Errorf("TotalTokens = %d, want 40 (should overwrite)", usage.TotalTokens)
		}
	})
}

func TestCopilotGatewayService_ParseNonStreamUsage(t *testing.T) {
	svc := &CopilotGatewayService{}

	t.Run("valid usage", func(t *testing.T) {
		body := []byte(`{"id":"chatcmpl-xxx","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
		usage := svc.parseNonStreamUsage(body)

		if usage.PromptTokens != 100 {
			t.Errorf("PromptTokens = %d, want 100", usage.PromptTokens)
		}
		if usage.CompletionTokens != 50 {
			t.Errorf("CompletionTokens = %d, want 50", usage.CompletionTokens)
		}
		if usage.TotalTokens != 150 {
			t.Errorf("TotalTokens = %d, want 150", usage.TotalTokens)
		}
	})

	t.Run("no usage", func(t *testing.T) {
		body := []byte(`{"id":"chatcmpl-xxx","choices":[]}`)
		usage := svc.parseNonStreamUsage(body)

		if usage.TotalTokens != 0 {
			t.Errorf("TotalTokens = %d, want 0", usage.TotalTokens)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		usage := svc.parseNonStreamUsage([]byte(`{invalid}`))
		if usage.TotalTokens != 0 {
			t.Errorf("TotalTokens = %d, want 0", usage.TotalTokens)
		}
	})
}

func TestCopilotGatewayService_HandleNonStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &CopilotGatewayService{}

	t.Run("success", func(t *testing.T) {
		respBody := `{"id":"chatcmpl-xxx","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`

		// Create mock upstream response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Request-Id": {"req-123"}},
			Body:       copilotStringReadCloser(respBody),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		result, err := svc.handleNonStreamingResponse(c, resp, "gpt-4o", time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
		}
		if result.Model != "gpt-4o" {
			t.Errorf("Model = %q, want %q", result.Model, "gpt-4o")
		}
		if result.Usage == nil {
			t.Fatal("Usage should not be nil")
		}
		if result.Usage.TotalTokens != 8 {
			t.Errorf("TotalTokens = %d, want 8", result.Usage.TotalTokens)
		}

		// Check response was forwarded to client
		if w.Code != http.StatusOK {
			t.Errorf("response code = %d, want %d", w.Code, http.StatusOK)
		}
		if !strings.Contains(w.Body.String(), "chatcmpl-xxx") {
			t.Errorf("response body should contain chatcmpl-xxx")
		}
	})
}

func TestCopilotGatewayService_HandleErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("401 invalidates token", func(t *testing.T) {
		provider := NewCopilotTokenProvider(nil)
		// Pre-populate token cache
		provider.tokens[42] = nil // just to verify it gets deleted

		svc := &CopilotGatewayService{tokenProvider: provider}

		errBody := `{"error":{"message":"Unauthorized","type":"auth_error"}}`
		resp := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       copilotStringReadCloser(errBody),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		account := &Account{ID: 42, Platform: PlatformCopilot}
		result, err := svc.handleErrorResponse(c, resp, account)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.StatusCode != http.StatusUnauthorized {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusUnauthorized)
		}

		// Verify token was invalidated
		provider.mu.RLock()
		_, exists := provider.tokens[42]
		provider.mu.RUnlock()
		if exists {
			t.Error("token should have been invalidated on 401")
		}
	})

	t.Run("429 forwards error", func(t *testing.T) {
		svc := &CopilotGatewayService{tokenProvider: NewCopilotTokenProvider(nil)}

		errBody := `{"error":{"message":"Rate limited"}}`
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       copilotStringReadCloser(errBody),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		account := &Account{ID: 1, Platform: PlatformCopilot}
		result, err := svc.handleErrorResponse(c, resp, account)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.StatusCode != http.StatusTooManyRequests {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusTooManyRequests)
		}
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("response code = %d, want %d", w.Code, http.StatusTooManyRequests)
		}
	})
}

func TestCopilotGatewayService_HandleStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &CopilotGatewayService{}

	t.Run("streams SSE lines", func(t *testing.T) {
		// Build a mock SSE response
		sseLines := []string{
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n",
			"\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n",
			"\n",
			"data: [DONE]\n",
			"\n",
		}
		sseBody := strings.Join(sseLines, "")

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       copilotStringReadCloser(sseBody),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		result, err := svc.handleStreamingResponse(c, resp, "gpt-4o", time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
		}
		if result.Model != "gpt-4o" {
			t.Errorf("Model = %q, want %q", result.Model, "gpt-4o")
		}
		if result.Usage == nil {
			t.Fatal("Usage should not be nil")
		}
		if result.Usage.TotalTokens != 7 {
			t.Errorf("TotalTokens = %d, want 7", result.Usage.TotalTokens)
		}

		// Verify SSE content was forwarded
		body := w.Body.String()
		if !strings.Contains(body, "Hello") {
			t.Error("response should contain 'Hello'")
		}
		if !strings.Contains(body, "[DONE]") {
			t.Error("response should contain '[DONE]'")
		}
	})
}

func TestCopilotGatewayService_ListModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success", func(t *testing.T) {
		modelsResp := `{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			auth := r.Header.Get("Authorization")
			if auth != "Bearer copilot-token-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, modelsResp)
		}))
		defer server.Close()

		provider := NewCopilotTokenProvider(nil)
		svc := NewCopilotGatewayService(provider)

		// Pre-populate token so no exchange is needed
		tok := newCopilotTestToken("copilot-token-123")
		provider.mu.Lock()
		provider.tokens[1] = &tok
		provider.mu.Unlock()

		account := &Account{
			ID:       1,
			Platform: PlatformCopilot,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"github_token": "ghp_test",
				"base_url":     server.URL,
			},
		}

		body, err := svc.ListModels(t.Context(), account)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(string(body), "gpt-4o") {
			t.Errorf("response should contain model list, got: %s", string(body))
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal"}`)
		}))
		defer server.Close()

		provider := NewCopilotTokenProvider(nil)
		svc := NewCopilotGatewayService(provider)

		tok := newCopilotTestToken("tok")
		provider.mu.Lock()
		provider.tokens[2] = &tok
		provider.mu.Unlock()

		account := &Account{
			ID:       2,
			Platform: PlatformCopilot,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"github_token": "ghp_test",
				"base_url":     server.URL,
			},
		}

		_, err := svc.ListModels(t.Context(), account)
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("error should mention status code, got: %v", err)
		}
	})
}

// ── helpers ──────────────────────────────────────────────────────────

// copilotStringReadCloser wraps a string as io.ReadCloser for http.Response.Body.
func copilotStringReadCloser(s string) *copilotTestReadCloser {
	return &copilotTestReadCloser{Reader: strings.NewReader(s)}
}

type copilotTestReadCloser struct {
	Reader *strings.Reader
}

func (rc *copilotTestReadCloser) Read(p []byte) (int, error) { return rc.Reader.Read(p) }
func (rc *copilotTestReadCloser) Close() error               { return nil }

// newCopilotTestToken returns a copilot.CopilotToken that won't expire during tests.
func newCopilotTestToken(token string) copilot.CopilotToken {
	return copilot.CopilotToken{
		Token:     token,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		RefreshAt: time.Now().Add(5 * time.Minute),
	}
}
