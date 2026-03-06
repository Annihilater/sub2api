package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// CopilotGatewayService handles forwarding requests to the GitHub Copilot API.
//
// It supports:
//   - /chat/completions (OpenAI-compatible format, streaming and non-streaming)
//   - /models (list available models)
//
// Authentication is handled via CopilotTokenProvider, which exchanges
// GitHub tokens for short-lived Copilot API tokens.
type CopilotGatewayService struct {
	tokenProvider *CopilotTokenProvider
	httpClient    *http.Client
}

// NewCopilotGatewayService creates a new CopilotGatewayService.
func NewCopilotGatewayService(
	tokenProvider *CopilotTokenProvider,
) *CopilotGatewayService {
	// Use HTTP/1.1 only to avoid 421 Misdirected Request errors.
	//
	// The Copilot API returns 421 when an HTTP/2 connection established for one
	// account token is subsequently reused with a different Bearer token.
	// Setting NextProtos to ["http/1.1"] prevents ALPN from negotiating HTTP/2
	// during TLS handshake, so every connection is HTTP/1.1.  ForceAttemptHTTP2
	// is also false to ensure the stdlib's http2 package never upgrades.
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Explicitly advertise only HTTP/1.1 during TLS ALPN negotiation.
			// Without this, Go's net/http will negotiate h2 even when
			// ForceAttemptHTTP2 is false, because the server supports h2.
			NextProtos: []string{"http/1.1"},
		},
		ForceAttemptHTTP2:   false,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &CopilotGatewayService{
		tokenProvider: tokenProvider,
		httpClient: &http.Client{
			Timeout:   5 * time.Minute, // long timeout for streaming
			Transport: transport,
		},
	}
}

// CopilotForwardResult holds the result of a Copilot API request.
type CopilotForwardResult struct {
	StatusCode      int
	Model           string
	Usage           *CopilotUsage
	Duration        time.Duration // 请求总耗时
	FirstTokenMs    *int          // 首token时间（流式请求，毫秒）
	ReasoningEffort *string       // 推理强度（Responses API 请求，如 "low"/"medium"/"high"）
}

// CopilotUsage tracks token usage from a Copilot API response.
type CopilotUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ForwardChatCompletions forwards a chat/completions request to the Copilot API.
func (s *CopilotGatewayService) ForwardChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*CopilotForwardResult, error) {
	startTime := time.Now()

	// Normalize the OpenAI request body before forwarding:
	// merge consecutive same-role messages so Copilot API doesn't reject with 400.
	// Claude Code (via cc-switch OpenAI mode) sends adjacent user messages
	// (e.g. <available-deferred-tools> + actual message) that Copilot rejects.
	body = mergeConsecutiveSameRoleMessagesInOpenAIBody(body)

	// Get Copilot API token
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot auth: %w", err)
	}

	// Determine base URL for /chat/completions.
	// Priority: explicit base_url credential (legacy) → plan_type credential → default.
	// The plan_type credential ("individual" / "business" / "enterprise") maps to the
	// corresponding subdomain; unknown or empty values fall back to CopilotAPIBase.
	baseURL := copilot.CopilotAPIBase
	if customURL := strings.TrimSpace(account.GetCredential("base_url")); customURL != "" {
		baseURL = strings.TrimRight(customURL, "/")
	} else if planType := strings.TrimSpace(account.GetCredential("plan_type")); planType != "" {
		baseURL = copilot.ChatBaseURLForPlan(planType)
	}

	// Extract model from request body for logging
	model := extractModelFromBody(body)

	// Detect streaming mode
	isStream := detectStreamMode(body)

	// Determine X-Initiator: "agent" for multi-turn conversations (has assistant/tool messages),
	// "user" for the first turn. Matches TypeScript copilot-api reference logic.
	initiator := copilotInitiator(body)

	// Build upstream request
	upstreamURL := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("copilot: build request: %w", err)
	}

	// Set Copilot-specific headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range copilot.CopilotHeaders(initiator, false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	// Send request
	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is from trusted Copilot API config
	if err != nil {
		return nil, fmt.Errorf("copilot: upstream request: %w", err)
	}

	slog.Debug("copilot upstream response",
		"account_id", account.ID,
		"model", model,
		"status", resp.StatusCode,
		"stream", isStream,
		"latency_ms", time.Since(startTime).Milliseconds())

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(c, resp, account)
	}

	// Handle streaming response
	if isStream {
		return s.handleStreamingResponse(c, resp, model, startTime)
	}

	// Handle non-streaming response
	return s.handleNonStreamingResponse(c, resp, model, startTime)
}

// handleStreamingResponse proxies SSE streaming from Copilot API to the client.
func (s *CopilotGatewayService) handleStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
	startTime time.Time,
) (*CopilotForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("copilot: response writer does not support flushing")
	}

	usage := &CopilotUsage{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var firstTokenMs *int

	for scanner.Scan() {
		line := scanner.Text()

		// Parse usage from SSE data
		if strings.HasPrefix(line, "data: ") {
			data := line[6:]
			if data != "[DONE]" {
				// Record first token time on first data chunk
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				s.parseStreamUsage(data, usage)
			}
		}

		// Forward line to client
		fmt.Fprintf(c.Writer, "%s\n", line)
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("copilot stream scanner error", "error", err)
	}

	return &CopilotForwardResult{
		StatusCode:   http.StatusOK,
		Model:        model,
		Usage:        usage,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
	}, nil
}

// handleNonStreamingResponse proxies a non-streaming response from Copilot API.
func (s *CopilotGatewayService) handleNonStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
	startTime time.Time,
) (*CopilotForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot: read response: %w", err)
	}

	// Extract usage
	usage := s.parseNonStreamUsage(body)

	// Forward response headers
	for k, vals := range resp.Header {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Data(http.StatusOK, "application/json", body)

	return &CopilotForwardResult{
		StatusCode: http.StatusOK,
		Model:      model,
		Usage:      usage,
		Duration:   time.Since(startTime),
	}, nil
}

// handleErrorResponse handles non-200 responses from the Copilot API.
//
// For most error codes the response body is forwarded to the client immediately.
// 421 Misdirected Request is a connection-level error (HTTP/2 connection reused
// for a different virtual host) — we must NOT write the 421 to the client yet,
// because the handler loop needs to failover to another account first.  After
// calling CloseIdleConnections the next request will use a fresh connection.
func (s *CopilotGatewayService) handleErrorResponse(
	c *gin.Context,
	resp *http.Response,
	account *Account,
) (*CopilotForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	slog.Warn("copilot upstream error",
		"account_id", account.ID,
		"status", resp.StatusCode,
		"body", string(body))

	// Handle specific error codes
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		// Token may have expired, invalidate cache
		s.tokenProvider.InvalidateToken(account.ID)
	case http.StatusTooManyRequests:
		// Rate limited — caller should handle retry/failover
	case http.StatusMisdirectedRequest:
		// 421 is a connection-level error (HTTP/2 connection reused across
		// different Bearer tokens / virtual hosts).  Flush idle connections so
		// the next attempt gets a fresh TCP+TLS connection, then return without
		// writing anything to the client — the handler loop will failover.
		if t, ok := s.httpClient.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
		return &CopilotForwardResult{StatusCode: resp.StatusCode}, nil
	}

	// Forward error to client
	c.Data(resp.StatusCode, "application/json", body)

	return &CopilotForwardResult{
		StatusCode: resp.StatusCode,
	}, nil
}

// copilotInitiator returns the value for the X-Initiator header.
//
// When the conversation already includes an assistant or tool message
// (multi-turn / sub-agent call), Copilot expects "agent" which draws from the
// standard quota instead of the premium-interaction quota.  For fresh user
// messages the value is "user".
//
// This matches the logic in the TypeScript copilot-api reference:
//
//	const isAgentCall = payload.messages.some(msg =>
//	  ["assistant", "tool"].includes(msg.role))
func copilotInitiator(openAIBody []byte) string {
	var req struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(openAIBody, &req); err != nil {
		return "user"
	}
	for _, m := range req.Messages {
		if m.Role == "assistant" || m.Role == "tool" {
			return "agent"
		}
	}
	return "user"
}

// extractModelFromBody reads the model field from a JSON request body.
func extractModelFromBody(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return req.Model
}

// detectStreamMode checks if the request body has "stream": true.
func detectStreamMode(body []byte) bool {
	var req struct {
		Stream any `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	switch v := req.Stream.(type) {
	case bool:
		return v
	default:
		return false
	}
}

// extractCopilotReasoningEffort extracts the reasoning_effort from a Responses API
// request body. Checks both "reasoning.effort" (nested) and flat "reasoning_effort".
// Returns nil if not present or not a recognized value.
func extractCopilotReasoningEffort(body []byte) *string {
	// Try nested: {"reasoning": {"effort": "high"}}
	effort := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if effort == "" {
		// Fallback: flat field {"reasoning_effort": "high"}
		effort = strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	}
	if effort == "" {
		return nil
	}
	// Normalize to lowercase.
	normalized := strings.ToLower(effort)
	return &normalized
}

// parseStreamUsage extracts usage data from an SSE data line.
// Handles two formats:
//
// Chat Completions format (most models):
//
//	{"usage": {"prompt_tokens": N, "completion_tokens": M}}
//
// Responses API format (Codex and similar):
//
//	{"type": "response.completed", "response": {"usage": {"input_tokens": N, "output_tokens": M}}}
func (s *CopilotGatewayService) parseStreamUsage(data string, usage *CopilotUsage) {
	b := []byte(data)

	// Try Chat Completions format first.
	var ccChunk struct {
		Usage *CopilotUsage `json:"usage"`
	}
	if err := json.Unmarshal(b, &ccChunk); err == nil && ccChunk.Usage != nil &&
		(ccChunk.Usage.PromptTokens > 0 || ccChunk.Usage.CompletionTokens > 0) {
		usage.PromptTokens = ccChunk.Usage.PromptTokens
		usage.CompletionTokens = ccChunk.Usage.CompletionTokens
		usage.TotalTokens = ccChunk.Usage.TotalTokens
		return
	}

	// Try Responses API format: type = "response.completed".
	var respChunk struct {
		Type     string `json:"type"`
		Response struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &respChunk); err == nil &&
		respChunk.Type == "response.completed" {
		usage.PromptTokens = respChunk.Response.Usage.InputTokens
		usage.CompletionTokens = respChunk.Response.Usage.OutputTokens
		usage.TotalTokens = respChunk.Response.Usage.InputTokens + respChunk.Response.Usage.OutputTokens
	}
}

// parseNonStreamUsage extracts usage data from a non-streaming response body.
// Handles both Chat Completions format (prompt_tokens/completion_tokens)
// and Responses API format (input_tokens/output_tokens).
func (s *CopilotGatewayService) parseNonStreamUsage(body []byte) *CopilotUsage {
	// Try Chat Completions format.
	var ccResp struct {
		Usage *CopilotUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &ccResp); err == nil && ccResp.Usage != nil &&
		(ccResp.Usage.PromptTokens > 0 || ccResp.Usage.CompletionTokens > 0) {
		return ccResp.Usage
	}

	// Try Responses API format.
	var respResp struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &respResp); err == nil &&
		(respResp.Usage.InputTokens > 0 || respResp.Usage.OutputTokens > 0) {
		return &CopilotUsage{
			PromptTokens:     respResp.Usage.InputTokens,
			CompletionTokens: respResp.Usage.OutputTokens,
			TotalTokens:      respResp.Usage.InputTokens + respResp.Usage.OutputTokens,
		}
	}

	return &CopilotUsage{}
}

// ListModels returns the list of models available on the Copilot API.
func (s *CopilotGatewayService) ListModels(
	ctx context.Context,
	account *Account,
) ([]byte, error) {
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot auth: %w", err)
	}

	baseURL := copilot.CopilotAPIBase
	if customURL := strings.TrimSpace(account.GetCredential("base_url")); customURL != "" {
		baseURL = strings.TrimRight(customURL, "/")
	} else if planType := strings.TrimSpace(account.GetCredential("plan_type")); planType != "" {
		baseURL = copilot.ChatBaseURLForPlan(planType)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("copilot: build models request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	for k, vals := range copilot.CopilotHeaders("user", false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is from trusted Copilot API config
	if err != nil {
		return nil, fmt.Errorf("copilot: models request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot: read models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot: models HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Deduplicate model IDs — the Copilot API may return the same model ID
	// more than once (e.g. gpt-4o appears twice in some responses).
	body = deduplicateModelsList(body)

	return body, nil
}

// deduplicateModelsList removes duplicate model entries (by id) from an
// OpenAI-format /models JSON response. The first occurrence of each ID is kept.
func deduplicateModelsList(body []byte) []byte {
	var resp struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}

	seen := make(map[string]struct{}, len(resp.Data))
	deduped := make([]json.RawMessage, 0, len(resp.Data))
	for _, raw := range resp.Data {
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &m); err != nil || m.ID == "" {
			deduped = append(deduped, raw)
			continue
		}
		if _, exists := seen[m.ID]; exists {
			continue
		}
		seen[m.ID] = struct{}{}
		deduped = append(deduped, raw)
	}

	resp.Data = deduped
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}

// OpenAI Responses API gateway (Codex CLI)
// ─────────────────────────────────────────────────────────────────────────────

// ForwardResponses forwards an OpenAI Responses API request to the Copilot API.
//
// Codex CLI uses wire_api = "responses" which sends requests to {base_url}/responses.
// The request and response formats are passed through as-is — no translation needed
// since both sides speak the same Responses API protocol.
func (s *CopilotGatewayService) ForwardResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*CopilotForwardResult, error) {
	startTime := time.Now()

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot auth: %w", err)
	}

	// The /responses endpoint is only available on the canonical api.githubcopilot.com.
	// Plan-specific subdomains (individual, business, enterprise) and any legacy
	// custom base_url that points to a subdomain do NOT expose /responses — they
	// return 421 Misdirected Request.  Always use CopilotAPIBase here regardless of
	// the account's plan_type or base_url setting.
	baseURL := copilot.CopilotAPIBase

	// Extract reasoning_effort before model rewrite (body still has original model).
	// Uses the same gjson-based approach as extractOpenAIReasoningEffortFromBody.
	reasoningEffort := extractCopilotReasoningEffort(body)

	// Apply model mapping and normalize model name (dash→dot for Claude models).
	model := extractModelFromBody(body)

	isStream := detectStreamMode(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("copilot responses: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range copilot.CopilotHeaders("user", false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is from trusted Copilot API config
	if err != nil {
		return nil, fmt.Errorf("copilot responses: upstream request: %w", err)
	}

	slog.Debug("copilot responses upstream response",
		"account_id", account.ID,
		"model", model,
		"status", resp.StatusCode,
		"stream", isStream,
		"latency_ms", time.Since(startTime).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(c, resp, account)
	}

	var result *CopilotForwardResult
	var fwdErr error
	if isStream {
		result, fwdErr = s.handleStreamingResponse(c, resp, model, startTime)
	} else {
		result, fwdErr = s.handleNonStreamingResponse(c, resp, model, startTime)
	}
	if fwdErr != nil || result == nil {
		return result, fwdErr
	}
	// Attach reasoning_effort extracted from request body.
	result.ReasoningEffort = reasoningEffort
	return result, nil
}

// Anthropic /v1/messages gateway
// ─────────────────────────────────────────────────────────────────────────────

// ForwardMessages receives an Anthropic /v1/messages request, translates it to
// OpenAI /chat/completions format, forwards it to the Copilot API, and
// translates the response back to Anthropic format.
//
// This allows Claude Code (and any Anthropic-compatible client) to use GitHub
// Copilot accounts as the backend without any client-side changes.
func (s *CopilotGatewayService) ForwardMessages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	anthropicBody []byte,
) (*CopilotForwardResult, error) {
	startTime := time.Now()

	// Detect streaming before translation (we need to know for the response path).
	isStream := detectAnthropicStream(anthropicBody)

	// Translate Anthropic request → OpenAI format.
	openAIBody, err := translateAnthropicToOpenAI(anthropicBody, nil)
	if err != nil {
		return nil, fmt.Errorf("copilot messages: translate request: %w", err)
	}

	// Extract model name for logging.
	model := extractModelFromBody(openAIBody)

	// DEBUG: log the full translated request body to diagnose 400 Bad Request
	slog.Debug("copilot messages translated openai body",
		"account_id", account.ID,
		"model", model,
		"body", string(openAIBody))

	// Get Copilot API token.
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot messages: auth: %w", err)
	}

	// Determine base URL.
	baseURL := copilot.CopilotAPIBase
	if customURL := strings.TrimSpace(account.GetCredential("base_url")); customURL != "" {
		baseURL = strings.TrimRight(customURL, "/")
	} else if planType := strings.TrimSpace(account.GetCredential("plan_type")); planType != "" {
		baseURL = copilot.ChatBaseURLForPlan(planType)
	}

	// Determine X-Initiator: "agent" when the conversation already has assistant
	// or tool turns (multi-turn / sub-agent call), "user" for the first turn.
	// This mirrors the TypeScript copilot-api reference implementation.
	initiator := copilotInitiator(openAIBody)

	// Build upstream request to Copilot /chat/completions.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(openAIBody))
	if err != nil {
		return nil, fmt.Errorf("copilot messages: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range copilot.CopilotHeaders(initiator, false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is from trusted Copilot API config
	if err != nil {
		return nil, fmt.Errorf("copilot messages: upstream request: %w", err)
	}

	slog.Debug("copilot messages upstream response",
		"account_id", account.ID,
		"model", model,
		"status", resp.StatusCode,
		"stream", isStream,
		"latency_ms", time.Since(startTime).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(c, resp, account)
	}

	if isStream {
		return s.handleMessagesStreamingResponse(c, resp, model, startTime)
	}
	return s.handleMessagesNonStreamingResponse(c, resp, model, startTime)
}

// handleMessagesNonStreamingResponse reads the OpenAI response and writes back
// an Anthropic-format JSON response.
func (s *CopilotGatewayService) handleMessagesNonStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
	startTime time.Time,
) (*CopilotForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot messages: read response: %w", err)
	}

	usage := s.parseNonStreamUsage(body)

	// Translate OpenAI response → Anthropic format.
	anthropicBody, err := translateOpenAIToAnthropic(body)
	if err != nil {
		slog.Warn("copilot messages: failed to translate response, forwarding raw",
			"error", err, "model", model)
		// Fall back to raw body so the client gets something.
		c.Data(http.StatusOK, "application/json", body)
		return &CopilotForwardResult{StatusCode: http.StatusOK, Model: model, Usage: usage, Duration: time.Since(startTime)}, nil
	}

	c.Data(http.StatusOK, "application/json", anthropicBody)
	return &CopilotForwardResult{
		StatusCode: http.StatusOK,
		Model:      model,
		Usage:      usage,
		Duration:   time.Since(startTime),
	}, nil
}

// handleMessagesStreamingResponse reads the Copilot SSE stream, translates each
// OpenAI chunk to Anthropic SSE events, and writes them to the client.
func (s *CopilotGatewayService) handleMessagesStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
	startTime time.Time,
) (*CopilotForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("copilot messages: response writer does not support flushing")
	}

	state := &copilotStreamState{
		toolCalls: make(map[int]copilotToolCallInfo),
	}
	usage := &CopilotUsage{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var firstTokenMs *int

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			// Forward blank lines / non-data lines as-is to maintain SSE framing.
			fmt.Fprintf(c.Writer, "%s\n", line)
			flusher.Flush()
			continue
		}

		data := line[6:]
		if data == "[DONE]" {
			// Anthropic clients don't expect [DONE]; just stop.
			break
		}

		var chunk openAIChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			slog.Debug("copilot messages stream: skip unparseable chunk", "data", data)
			continue
		}

		// Record first token time on first parseable chunk.
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		// Accumulate usage.
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
			usage.TotalTokens = chunk.Usage.TotalTokens
		}

		// Translate chunk → Anthropic events.
		events := translateChunkToAnthropicEvents(&chunk, state)
		for _, evt := range events {
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", extractEventType(evt), evt)
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("copilot messages stream scanner error", "error", err)
	}

	return &CopilotForwardResult{
		StatusCode:   http.StatusOK,
		Model:        model,
		Usage:        usage,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
	}, nil
}

// detectAnthropicStream checks if an Anthropic request body has "stream": true.
func detectAnthropicStream(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

// extractEventType reads the "type" field from a JSON event object for the SSE
// event name (e.g. "message_start", "content_block_delta", …).
func extractEventType(jsonStr string) string {
	var e struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &e); err == nil && e.Type != "" {
		return e.Type
	}
	return "message"
}

// copilotInternalUserResponse is the raw response from the GitHub
// copilot_internal/user endpoint. Only the fields we need are decoded.
type copilotInternalUserResponse struct {
	// CopilotPlan is the plan type string returned by GitHub.
	CopilotPlan string `json:"copilot_plan"`

	// ChatEnabled indicates whether chat is available.
	ChatEnabled bool `json:"chat_enabled"`

	// Login is the GitHub username.
	Login string `json:"login"`

	// QuotaResetDate is the date the quota resets (top-level field).
	QuotaResetDate string `json:"quota_reset_date,omitempty"`

	// QuotaSnapshots is the rich quota data returned by GitHub (newer format).
	QuotaSnapshots *struct {
		Completions         *copilotQuotaSnapshotRaw `json:"completions"`
		Chat                *copilotQuotaSnapshotRaw `json:"chat"`
		PremiumInteractions *copilotQuotaSnapshotRaw `json:"premium_interactions"`
	} `json:"quota_snapshots"`
}

// copilotQuotaSnapshotRaw mirrors the quota_snapshots entries from GitHub API.
type copilotQuotaSnapshotRaw struct {
	Entitlement      int     `json:"entitlement"`
	OverageCount     int     `json:"overage_count"`
	OveragePermitted bool    `json:"overage_permitted"`
	PercentRemaining float64 `json:"percent_remaining"`
	Remaining        int     `json:"remaining"`
	Unlimited        bool    `json:"unlimited"`
}

// toQuotaDetail converts a raw snapshot into the canonical QuotaDetail.
func (r *copilotQuotaSnapshotRaw) toQuotaDetail() *copilot.QuotaDetail {
	if r == nil {
		return nil
	}
	return &copilot.QuotaDetail{
		Entitlement:      r.Entitlement,
		OveragePermitted: r.OveragePermitted,
		Used:             r.Entitlement - r.Remaining + r.OverageCount,
		Unlimited:        r.Unlimited,
		Remaining:        r.Remaining,
		OverageCount:     r.OverageCount,
		PercentRemaining: r.PercentRemaining,
	}
}

// FetchQuota fetches the Copilot quota and plan information for an account from
// the GitHub copilot_internal/user API endpoint.
func (s *CopilotGatewayService) FetchQuota(
	ctx context.Context,
	account *Account,
) (*copilot.CopilotQuotaInfo, error) {
	githubToken := account.GetCredential("github_token")
	if githubToken == "" {
		return nil, fmt.Errorf("copilot: no github_token configured for account %d", account.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/copilot_internal/user", nil)
	if err != nil {
		return nil, fmt.Errorf("copilot: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", copilot.DefaultGitHubAPIVersion)

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is from trusted Copilot API config
	if err != nil {
		return nil, fmt.Errorf("copilot: quota request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot: read quota response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot: quota HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw copilotInternalUserResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("copilot: parse quota response: %w", err)
	}

	// Map plan string to human-readable plan type.
	planType := planTypeFromString(raw.CopilotPlan)

	info := &copilot.CopilotQuotaInfo{
		Plan:           raw.CopilotPlan,
		PlanType:       planType,
		QuotaResetDate: raw.QuotaResetDate,
	}

	if s := raw.QuotaSnapshots; s != nil {
		info.Completions = s.Completions.toQuotaDetail()
		info.Chat = s.Chat.toQuotaDetail()
		info.PremiumInteractions = s.PremiumInteractions.toQuotaDetail()
	}

	return info, nil
}

// FetchAllCopilotQuotas fetches quota info for all active Copilot accounts
// concurrently. Results are returned in arbitrary order. Failures are
// recorded in the Error field of the corresponding summary entry.
func (s *CopilotGatewayService) FetchAllCopilotQuotas(
	ctx context.Context,
	adminSvc AdminService,
) ([]copilot.CopilotAccountQuotaSummary, error) {
	// Page through all Copilot accounts (large page size to minimise round-trips).
	const pageSize = 200
	var allAccounts []Account
	for page := 1; ; page++ {
		accounts, _, err := adminSvc.ListAccounts(ctx, page, pageSize, PlatformCopilot, "", StatusActive, "", 0)
		if err != nil {
			return nil, fmt.Errorf("copilot: list accounts: %w", err)
		}
		allAccounts = append(allAccounts, accounts...)
		if len(accounts) < pageSize {
			break
		}
	}

	results := make([]copilot.CopilotAccountQuotaSummary, len(allAccounts))
	var wg sync.WaitGroup
	for i, acc := range allAccounts {
		wg.Add(1)
		go func(idx int, a Account) {
			defer wg.Done()
			summary := copilot.CopilotAccountQuotaSummary{
				AccountID:   a.ID,
				AccountName: a.Name,
				Status:      string(a.Status),
				GitHubLogin: a.GetCredential("github_login"),
			}
			qi, err := s.FetchQuota(ctx, &a)
			if err != nil {
				summary.Error = err.Error()
			} else {
				summary.QuotaInfo = qi
			}
			results[idx] = summary
		}(i, acc)
	}
	wg.Wait()
	return results, nil
}

// mergeConsecutiveSameRoleMessagesInOpenAIBody parses an OpenAI Chat Completions
// request body, merges adjacent messages that share the same "user" or "system"
// role, and returns the re-serialised body.
//
// Claude Code in OpenAI-mode (via cc-switch) sends consecutive user messages
// (e.g. <available-deferred-tools> injection + actual user message).  The
// Copilot API rejects adjacent same-role messages with 400 Bad Request.
// This function normalises the body before forwarding, matching the behaviour
// already applied on the Anthropic path via mergeConsecutiveSameRoleMessages.
//
// If parsing fails the original body is returned unchanged so the upstream
// error (if any) is forwarded to the caller.
func mergeConsecutiveSameRoleMessagesInOpenAIBody(body []byte) []byte {
	var req struct {
		Messages []struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
		} `json:"messages"`
	}

	// We want to preserve all other fields (model, stream, temperature …),
	// so unmarshal into a generic map and only rewrite the messages field.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(body, &generic); err != nil {
		return body
	}

	msgsRaw, ok := generic["messages"]
	if !ok {
		return body
	}
	if err := json.Unmarshal(msgsRaw, &req.Messages); err != nil {
		return body
	}
	if len(req.Messages) == 0 {
		return body
	}

	// Build merged messages list.
	type msg struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	}

	merged := make([]msg, 0, len(req.Messages))
	for _, m := range req.Messages {
		if len(merged) == 0 {
			merged = append(merged, msg{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls})
			continue
		}
		prev := &merged[len(merged)-1]

		// Only merge user/system; never merge assistant/tool messages, and
		// never merge a message that has tool_calls.
		canMerge := m.Role == prev.Role &&
			(m.Role == "user" || m.Role == "system") &&
			len(m.ToolCalls) == 0 &&
			len(prev.ToolCalls) == 0

		if !canMerge {
			merged = append(merged, msg{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls})
			continue
		}

		// Extract text from both sides and join.
		prevText := extractTextFromOpenAIContent(prev.Content)
		curText := extractTextFromOpenAIContent(m.Content)
		combined := prevText
		if curText != "" {
			if combined != "" {
				combined += "\n\n"
			}
			combined += curText
		}
		combinedJSON, err := json.Marshal(combined)
		if err != nil {
			merged = append(merged, msg{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls})
			continue
		}
		prev.Content = combinedJSON
	}

	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return body
	}
	generic["messages"] = mergedJSON

	out, err := json.Marshal(generic)
	if err != nil {
		return body
	}
	return out
}

// extractTextFromOpenAIContent extracts a plain string from an OpenAI message
// content field, which may be a JSON string or a JSON array of content parts.
func extractTextFromOpenAIContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content parts: [{"type":"text","text":"..."}].
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if p.Type == "text" && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// planTypeFromString returns a human-readable plan type label.
func planTypeFromString(plan string) string {
	switch plan {
	case "copilot_for_individuals", "copilot_individual":
		return "Individual"
	case "copilot_business":
		return "Business"
	case "copilot_enterprise":
		return "Enterprise"
	default:
		if plan != "" {
			return plan
		}
		return "Unknown"
	}
}
