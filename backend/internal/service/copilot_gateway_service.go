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
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
	"github.com/gin-gonic/gin"
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

	return body, nil
}
