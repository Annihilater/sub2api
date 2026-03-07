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

func TestCopilotInitiator(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"first user turn – no history",
			`{"messages":[{"role":"user","content":"hi"}]}`,
			"user",
		},
		{
			"multi-turn with assistant – agent call",
			`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"more"}]}`,
			"agent",
		},
		{
			"tool result message – agent call",
			`{"messages":[{"role":"user","content":"hi"},{"role":"tool","content":"result"}]}`,
			"agent",
		},
		{
			"system + user only",
			`{"messages":[{"role":"system","content":"you are helpful"},{"role":"user","content":"hi"}]}`,
			"user",
		},
		{
			"invalid json defaults to user",
			`{invalid`,
			"user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := copilotInitiator([]byte(tt.body))
			if got != tt.want {
				t.Errorf("copilotInitiator() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
		provider := NewCopilotTokenProvider()
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

	t.Run("429 signals failover without writing to client", func(t *testing.T) {
		svc := &CopilotGatewayService{tokenProvider: NewCopilotTokenProvider()}

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

		// 429 must NOT be written to the client — handler loop needs to failover.
		if result.StatusCode != http.StatusTooManyRequests {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusTooManyRequests)
		}
		// Default recorder code is 200 (nothing written).
		if w.Code != http.StatusOK {
			t.Errorf("response code = %d, want 200 (no write to client for 429)", w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("response body should be empty for 429, got: %s", w.Body.String())
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
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

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

		provider := NewCopilotTokenProvider()
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

		provider := NewCopilotTokenProvider()
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

// ── OpenAI body merge ─────────────────────────────────────────────────────────

func TestMergeConsecutiveSameRoleMessagesInOpenAIBody(t *testing.T) {
	t.Run("consecutive user messages merged", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4.6","stream":true,"messages":[
			{"role":"user","content":"<available-deferred-tools>\nAgent\n</available-deferred-tools>"},
			{"role":"user","content":"hello world"}
		]}`
		got := mergeConsecutiveSameRoleMessagesInOpenAIBody([]byte(body))

		var result struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(got, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if result.Model != "claude-sonnet-4.6" {
			t.Errorf("model = %q, want claude-sonnet-4.6 (other fields preserved)", result.Model)
		}
		userCount := 0
		for _, m := range result.Messages {
			if m.Role == "user" {
				userCount++
			}
		}
		if userCount != 1 {
			t.Errorf("expected 1 merged user message, got %d: %v", userCount, result.Messages)
		}
		if len(result.Messages) > 0 && !strings.Contains(result.Messages[0].Content, "available-deferred-tools") {
			t.Errorf("merged content missing first part: %q", result.Messages[0].Content)
		}
		if len(result.Messages) > 0 && !strings.Contains(result.Messages[0].Content, "hello world") {
			t.Errorf("merged content missing second part: %q", result.Messages[0].Content)
		}
	})

	t.Run("alternating roles not merged", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[
			{"role":"user","content":"hi"},
			{"role":"assistant","content":"hello"},
			{"role":"user","content":"again"}
		]}`
		got := mergeConsecutiveSameRoleMessagesInOpenAIBody([]byte(body))

		var result struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(got, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(result.Messages) != 3 {
			t.Errorf("expected 3 messages (no merge), got %d", len(result.Messages))
		}
	})

	t.Run("content parts array merged as text", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[
			{"role":"user","content":[{"type":"text","text":"part one"}]},
			{"role":"user","content":"part two"}
		]}`
		got := mergeConsecutiveSameRoleMessagesInOpenAIBody([]byte(body))

		var result struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(got, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(result.Messages) != 1 {
			t.Errorf("expected 1 merged message, got %d", len(result.Messages))
		}
		if !strings.Contains(result.Messages[0].Content, "part one") ||
			!strings.Contains(result.Messages[0].Content, "part two") {
			t.Errorf("merged content = %q, want both parts", result.Messages[0].Content)
		}
	})

	t.Run("invalid json returned unchanged", func(t *testing.T) {
		body := []byte(`{invalid json`)
		got := mergeConsecutiveSameRoleMessagesInOpenAIBody(body)
		if string(got) != string(body) {
			t.Errorf("expected original body returned on parse error")
		}
	})

	t.Run("other fields preserved", func(t *testing.T) {
		body := `{"model":"gpt-4o","stream":true,"temperature":0.7,"messages":[
			{"role":"user","content":"hi"}
		]}`
		got := mergeConsecutiveSameRoleMessagesInOpenAIBody([]byte(body))

		var result struct {
			Model       string  `json:"model"`
			Stream      bool    `json:"stream"`
			Temperature float64 `json:"temperature"`
		}
		if err := json.Unmarshal(got, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if result.Model != "gpt-4o" || !result.Stream || result.Temperature != 0.7 {
			t.Errorf("fields not preserved: %+v", result)
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
