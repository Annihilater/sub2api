package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
	"github.com/gin-gonic/gin"
)

// claudeModelDotPattern matches Claude model IDs with dot-separated versions,
// e.g. "claude-sonnet-4.5", "claude-opus-4.6", "claude-haiku-4.5".
// Used to rewrite these for clients that expect dash-separated versions.
var claudeModelDotPattern = regexp.MustCompile(`claude-(?:sonnet|opus|haiku)-\d+\.\d+`)

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
	return &CopilotGatewayService{
		tokenProvider: tokenProvider,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // long timeout for streaming
		},
	}
}

// CopilotForwardResult holds the result of a Copilot API request.
type CopilotForwardResult struct {
	StatusCode int
	Model      string
	Usage      *CopilotUsage
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

	// Get Copilot API token
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot auth: %w", err)
	}

	// Determine base URL
	baseURL := copilot.CopilotAPIBase
	if customURL := strings.TrimSpace(account.GetCredential("base_url")); customURL != "" {
		baseURL = strings.TrimRight(customURL, "/")
	}

	// Apply model mapping if configured
	body, model := s.applyModelMapping(body, account)

	// Detect streaming mode
	isStream := detectStreamMode(body)

	// Build upstream request
	upstreamURL := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("copilot: build request: %w", err)
	}

	// Set Copilot-specific headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range copilot.CopilotHeaders("user", false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	// Send request
	resp, err := s.httpClient.Do(req)
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
		return s.handleStreamingResponse(c, resp, model)
	}

	// Handle non-streaming response
	return s.handleNonStreamingResponse(c, resp, model)
}

// handleStreamingResponse proxies SSE streaming from Copilot API to the client.
func (s *CopilotGatewayService) handleStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
) (*CopilotForwardResult, error) {
	defer resp.Body.Close()

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

	for scanner.Scan() {
		line := scanner.Text()

		// Parse usage from SSE data
		if strings.HasPrefix(line, "data: ") {
			data := line[6:]
			if data != "[DONE]" {
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
		StatusCode: http.StatusOK,
		Model:      model,
		Usage:      usage,
	}, nil
}

// handleNonStreamingResponse proxies a non-streaming response from Copilot API.
func (s *CopilotGatewayService) handleNonStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
) (*CopilotForwardResult, error) {
	defer resp.Body.Close()

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
	}, nil
}

// handleErrorResponse handles non-200 responses from the Copilot API.
func (s *CopilotGatewayService) handleErrorResponse(
	c *gin.Context,
	resp *http.Response,
	account *Account,
) (*CopilotForwardResult, error) {
	defer resp.Body.Close()

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
	}

	// Forward error to client
	c.Data(resp.StatusCode, "application/json", body)

	return &CopilotForwardResult{
		StatusCode: resp.StatusCode,
	}, nil
}

// applyModelMapping applies model mapping from account configuration.
func (s *CopilotGatewayService) applyModelMapping(body []byte, account *Account) ([]byte, string) {
	// Extract model from request body
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		return body, ""
	}

	originalModel := req.Model
	mappedModel := account.GetMappedModel(originalModel)

	if mappedModel != originalModel {
		// Replace model in request body
		newBody, err := json.Marshal(map[string]json.RawMessage{})
		if err == nil {
			// Simple approach: replace model field in the JSON
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(body, &raw); err == nil {
				modelBytes, _ := json.Marshal(mappedModel)
				raw["model"] = modelBytes
				if replaced, err := json.Marshal(raw); err == nil {
					newBody = replaced
					slog.Debug("copilot model mapping",
						"original", originalModel,
						"mapped", mappedModel)
					return newBody, originalModel
				}
			}
		}
	}

	return body, originalModel
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

// parseStreamUsage extracts usage data from an SSE data line.
func (s *CopilotGatewayService) parseStreamUsage(data string, usage *CopilotUsage) {
	var chunk struct {
		Usage *CopilotUsage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Usage != nil {
		usage.PromptTokens = chunk.Usage.PromptTokens
		usage.CompletionTokens = chunk.Usage.CompletionTokens
		usage.TotalTokens = chunk.Usage.TotalTokens
	}
}

// parseNonStreamUsage extracts usage data from a non-streaming response body.
func (s *CopilotGatewayService) parseNonStreamUsage(body []byte) *CopilotUsage {
	var resp struct {
		Usage *CopilotUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Usage != nil {
		return resp.Usage
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

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot: models request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot: read models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot: models HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Rewrite model IDs: replace dots with dashes in Claude model names so that
	// Claude Code's built-in model whitelist accepts them.
	// e.g. "claude-sonnet-4.5" → "claude-sonnet-4-5"
	// The reverse mapping is applied in normalizeCopilotModel when forwarding requests.
	body = rewriteModelIDsForClient(body)

	// Apply account-level model mapping to the model list so that users always
	// see the mapped (standardized) model IDs instead of the raw Copilot IDs.
	// e.g. if the account maps "claude-sonnet-4-5" → "claude-sonnet-4-5-20250929",
	// the returned model list will show the mapped ID.
	modelMapping := account.GetModelMapping()
	if len(modelMapping) > 0 {
		body = applyMappingToModelsList(body, modelMapping)
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

// applyMappingToModelsList rewrites model IDs in an OpenAI-format /models JSON
// response according to the provided mapping table.
//
// Only the "id" field of each model object is rewritten; all other fields are
// preserved as-is. Models whose IDs are not present in the mapping are returned
// unchanged.
func applyMappingToModelsList(body []byte, modelMapping map[string]string) []byte {
	// Parse the OpenAI models response: {"object":"list","data":[{"id":"..."},...]}
	var resp struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Not parseable — return as-is rather than breaking the response.
		return body
	}

	rewritten := make([]json.RawMessage, 0, len(resp.Data))
	for _, raw := range resp.Data {
		// Extract the model ID.
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &m); err != nil || m.ID == "" {
			rewritten = append(rewritten, raw)
			continue
		}

		mapped, ok := modelMapping[m.ID]
		if !ok || mapped == "" {
			rewritten = append(rewritten, raw)
			continue
		}

		// Replace the "id" (and "display_name" if present) in the raw JSON object.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			rewritten = append(rewritten, raw)
			continue
		}
		idBytes, _ := json.Marshal(mapped)
		obj["id"] = idBytes
		// Update display_name to match the mapped ID for clarity.
		obj["display_name"] = idBytes

		replaced, err := json.Marshal(obj)
		if err != nil {
			rewritten = append(rewritten, raw)
			continue
		}
		rewritten = append(rewritten, replaced)
	}

	resp.Data = rewritten
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}

// rewriteModelIDsForClient rewrites Claude model IDs in a Copilot /models JSON
// response, replacing dots with dashes so that Claude Code's built-in model
// whitelist accepts them.
//
// e.g. "claude-sonnet-4.5" → "claude-sonnet-4-5"
//
// Only Claude model IDs are rewritten; GPT and other models are left unchanged.
func rewriteModelIDsForClient(body []byte) []byte {
	// Simple string replacement on the raw JSON — fast and avoids full parse/re-encode.
	// We replace patterns like "claude-xxx-N.M" → "claude-xxx-N-M".
	result := claudeModelDotPattern.ReplaceAllFunc(body, func(match []byte) []byte {
		return bytes.ReplaceAll(match, []byte{'.'}, []byte{'-'})
	})
	return result
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
	openAIBody, err := translateAnthropicToOpenAI(anthropicBody, account.GetModelMapping())
	if err != nil {
		return nil, fmt.Errorf("copilot messages: translate request: %w", err)
	}

	// Apply model mapping (operates on the already-translated OpenAI body).
	openAIBody, model := s.applyModelMapping(openAIBody, account)

	// Get Copilot API token.
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("copilot messages: auth: %w", err)
	}

	// Determine base URL.
	baseURL := copilot.CopilotAPIBase
	if customURL := strings.TrimSpace(account.GetCredential("base_url")); customURL != "" {
		baseURL = strings.TrimRight(customURL, "/")
	}

	// Build upstream request to Copilot /chat/completions.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(openAIBody))
	if err != nil {
		return nil, fmt.Errorf("copilot messages: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range copilot.CopilotHeaders("user", false) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.httpClient.Do(req)
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
		return s.handleMessagesStreamingResponse(c, resp, model)
	}
	return s.handleMessagesNonStreamingResponse(c, resp, model)
}

// handleMessagesNonStreamingResponse reads the OpenAI response and writes back
// an Anthropic-format JSON response.
func (s *CopilotGatewayService) handleMessagesNonStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
) (*CopilotForwardResult, error) {
	defer resp.Body.Close()

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
		return &CopilotForwardResult{StatusCode: http.StatusOK, Model: model, Usage: usage}, nil
	}

	c.Data(http.StatusOK, "application/json", anthropicBody)
	return &CopilotForwardResult{
		StatusCode: http.StatusOK,
		Model:      model,
		Usage:      usage,
	}, nil
}

// handleMessagesStreamingResponse reads the Copilot SSE stream, translates each
// OpenAI chunk to Anthropic SSE events, and writes them to the client.
func (s *CopilotGatewayService) handleMessagesStreamingResponse(
	c *gin.Context,
	resp *http.Response,
	model string,
) (*CopilotForwardResult, error) {
	defer resp.Body.Close()

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
		StatusCode: http.StatusOK,
		Model:      model,
		Usage:      usage,
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

	// CopilotQuotaDetails contains fine-grained quota information when available.
	CopilotQuotaDetails *struct {
		Completions         *copilotQuotaDetailRaw `json:"completions"`
		Chat                *copilotQuotaDetailRaw `json:"chat"`
		PremiumInteractions *copilotQuotaDetailRaw `json:"premium_interactions"`
		QuotaResetDate      string                 `json:"quota_reset_date,omitempty"`
	} `json:"copilot_quota_details"`
}

type copilotQuotaDetailRaw struct {
	Entitlement      int  `json:"entitlement,omitempty"`
	OveragePermitted bool `json:"overage_permitted,omitempty"`
	Used             int  `json:"used,omitempty"`
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

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot: quota request: %w", err)
	}
	defer resp.Body.Close()

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
		Plan:     raw.CopilotPlan,
		PlanType: planType,
	}

	if d := raw.CopilotQuotaDetails; d != nil {
		info.QuotaResetDate = d.QuotaResetDate
		if d.Completions != nil {
			info.Completions = &copilot.QuotaDetail{
				Entitlement:      d.Completions.Entitlement,
				OveragePermitted: d.Completions.OveragePermitted,
				Used:             d.Completions.Used,
			}
		}
		if d.Chat != nil {
			info.Chat = &copilot.QuotaDetail{
				Entitlement:      d.Chat.Entitlement,
				OveragePermitted: d.Chat.OveragePermitted,
				Used:             d.Chat.Used,
			}
		}
		if d.PremiumInteractions != nil {
			info.PremiumInteractions = &copilot.QuotaDetail{
				Entitlement:      d.PremiumInteractions.Entitlement,
				OveragePermitted: d.PremiumInteractions.OveragePermitted,
				Used:             d.PremiumInteractions.Used,
			}
		}
	}

	return info, nil
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
