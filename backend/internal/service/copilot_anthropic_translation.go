package service

// copilot_anthropic_translation.go
//
// Implements Anthropic Messages API ↔ OpenAI Chat Completions translation
// for the GitHub Copilot gateway.
//
// Translation direction:
//   Incoming:  Anthropic /v1/messages  →  OpenAI /chat/completions  →  Copilot API
//   Outgoing:  Copilot API response    →  OpenAI response           →  Anthropic response
//
// Reference implementation: https://github.com/ericc-ch/copilot-api (TypeScript)

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Anthropic request types
// ─────────────────────────────────────────────────────────────────────────────

// AnthropicMessagesRequest is the body of a POST /v1/messages request.
type AnthropicMessagesRequest struct {
	Model         string              `json:"model"`
	Messages      []AnthropicMessage  `json:"messages"`
	MaxTokens     int                 `json:"max_tokens"`
	System        json.RawMessage     `json:"system,omitempty"` // string or []AnthropicTextBlock
	Metadata      *AnthropicMetadata  `json:"metadata,omitempty"`
	StopSequences []string            `json:"stop_sequences,omitempty"`
	Stream        bool                `json:"stream,omitempty"`
	Temperature   *float64            `json:"temperature,omitempty"`
	TopP          *float64            `json:"top_p,omitempty"`
	Tools         []AnthropicTool     `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice `json:"tool_choice,omitempty"`
}

type AnthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// AnthropicMessage is a single turn in the conversation.
type AnthropicMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"` // string or []block
}

// AnthropicTextBlock is a plain text content block.
type AnthropicTextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// AnthropicImageBlock is a base64-encoded image block.
type AnthropicImageBlock struct {
	Type   string                    `json:"type"` // "image"
	Source AnthropicImageBlockSource `json:"source"`
}

// AnthropicImageBlockSource holds image data.
type AnthropicImageBlockSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg" etc.
	Data      string `json:"data"`
}

// AnthropicToolResultBlock is the result of a tool call.
type AnthropicToolResultBlock struct {
	Type      string `json:"type"`        // "tool_result"
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// AnthropicToolUseBlock represents the model calling a tool.
type AnthropicToolUseBlock struct {
	Type  string          `json:"type"` // "tool_use"
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// AnthropicThinkingBlock represents a thinking/reasoning block (not used in Copilot).
type AnthropicThinkingBlock struct {
	Type     string `json:"type"` // "thinking"
	Thinking string `json:"thinking"`
}

// AnthropicTool is a tool definition in the request.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// AnthropicToolChoice specifies how the model should use tools.
type AnthropicToolChoice struct {
	Type string `json:"type"` // "auto" | "any" | "tool" | "none"
	Name string `json:"name,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Anthropic response types
// ─────────────────────────────────────────────────────────────────────────────

// AnthropicMessagesResponse is the response body for a non-streaming request.
type AnthropicMessagesResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`    // "message"
	Role         string                   `json:"role"`    // "assistant"
	Model        string                   `json:"model"`
	Content      []json.RawMessage        `json:"content"` // []AnthropicTextBlock | []AnthropicToolUseBlock
	StopReason   string                   `json:"stop_reason"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        AnthropicUsage           `json:"usage"`
}

// AnthropicUsage holds token usage counts.
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenAI (Copilot API) types
// ─────────────────────────────────────────────────────────────────────────────

// openAIChatRequest is the body sent to Copilot's /chat/completions.
type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stop        []string         `json:"stop,omitempty"`
	Stream      bool             `json:"stream"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	User        string           `json:"user,omitempty"`
	Tools       []openAITool     `json:"tools,omitempty"`
	ToolChoice  any              `json:"tool_choice,omitempty"`
}

// openAIMessage is a single message in an OpenAI chat request.
type openAIMessage struct {
	Role       string           `json:"role"` // system | user | assistant | tool
	Content    any              `json:"content"` // string or []openAIContentPart
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIContentPart is a part of a multi-modal message.
type openAIContentPart struct {
	Type     string                `json:"type"` // "text" | "image_url"
	Text     string                `json:"text,omitempty"`
	ImageURL *openAIImageURLObject `json:"image_url,omitempty"`
}

// openAIImageURLObject holds a base64-encoded image URL.
type openAIImageURLObject struct {
	URL string `json:"url"`
}

// openAITool is a function tool definition.
type openAITool struct {
	Type     string          `json:"type"` // "function"
	Function openAIFunction  `json:"function"`
}

// openAIFunction describes a callable function.
type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// openAIToolCall is a tool call made by the model.
type openAIToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"` // "function"
	Function openAIFunctionCall  `json:"function"`
}

// openAIFunctionCall is the function being called.
type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIFunctionCallDelta is an incremental update to a function call during streaming.
type openAIFunctionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// openAIToolCallDelta is an incremental tool call chunk during streaming.
type openAIToolCallDelta struct {
	Index    int                     `json:"index"`
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type,omitempty"`
	Function openAIFunctionCallDelta `json:"function,omitempty"`
}

// openAIChatResponse is a non-streaming response from Copilot.
type openAIChatResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Model   string          `json:"model"`
	Choices []openAIChoice  `json:"choices"`
	Usage   *openAIUsage    `json:"usage,omitempty"`
}

// openAIChoice is a single completion choice.
type openAIChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// openAIUsage holds token usage from an OpenAI response.
type openAIUsage struct {
	PromptTokens     int                  `json:"prompt_tokens"`
	CompletionTokens int                  `json:"completion_tokens"`
	TotalTokens      int                  `json:"total_tokens"`
	PromptDetails    *openAIPromptDetails `json:"prompt_tokens_details,omitempty"`
}

// openAIPromptDetails holds details about prompt tokens.
type openAIPromptDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// openAIChatStreamChunk is a single SSE chunk in a streaming response.
type openAIChatStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Model   string              `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage        `json:"usage,omitempty"`
}

// openAIStreamChoice is a choice delta in a streaming chunk.
type openAIStreamChoice struct {
	Index        int            `json:"index"`
	Delta        openAIDelta    `json:"delta"`
	FinishReason string         `json:"finish_reason"`
}

// openAIDelta is the incremental content in a stream chunk.
type openAIDelta struct {
	Role      string               `json:"role,omitempty"`
	Content   string               `json:"content,omitempty"`
	ToolCalls []openAIToolCallDelta `json:"tool_calls,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Request translation: Anthropic → OpenAI
// ─────────────────────────────────────────────────────────────────────────────

// translateAnthropicToOpenAI converts an Anthropic /v1/messages request body to
// an OpenAI /chat/completions request body suitable for the Copilot API.
// modelMapping is an optional account-level model mapping (may be nil); when
// a mapping entry is found for the requested model it takes priority over the
// generic dash-to-dot conversion.
func translateAnthropicToOpenAI(body []byte, modelMapping map[string]string) ([]byte, error) {
	var req AnthropicMessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}

	openAIReq := openAIChatRequest{
		Model:       normalizeCopilotModel(req.Model, modelMapping),
		Messages:    buildOpenAIMessages(req),
		MaxTokens:   req.MaxTokens,
		Stop:        req.StopSequences,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Tools:       translateTools(req.Tools),
		ToolChoice:  translateToolChoice(req.ToolChoice),
	}
	if req.Metadata != nil {
		openAIReq.User = req.Metadata.UserID
	}

	return json.Marshal(openAIReq)
}

// normalizeCopilotModel maps Anthropic-style model names (dashes) to the exact
// Copilot API model IDs (dots).
//
// Claude Code sends model names like "claude-sonnet-4-5" (dashes, as exposed
// by our /v1/models endpoint). Copilot API expects "claude-sonnet-4.5" (dots).
//
// The optional modelMapping parameter provides account-level overrides. When a
// mapping entry is found for the requested model, it takes priority over the
// generic conversion.
//
// Examples (no mapping):
//
//	"claude-sonnet-4-5"          → "claude-sonnet-4.5"
//	"claude-sonnet-4-5-20250929" → "claude-sonnet-4.5"
//	"claude-opus-4-6"            → "claude-opus-4.6"
//	"claude-haiku-4-5"           → "claude-haiku-4.5"
//	"gpt-4o"                     → "gpt-4o"  (unchanged)
func normalizeCopilotModel(model string, modelMapping map[string]string) string {
	// Check account-level mapping first.
	if len(modelMapping) > 0 {
		if target, ok := modelMapping[model]; ok {
			return target
		}
	}

	// Fallback: generic dash-to-dot conversion for known Claude model prefixes.
	prefixes := []string{
		"claude-sonnet-4-",
		"claude-opus-4-",
		"claude-haiku-4-",
		"claude-sonnet-3-",
		"claude-opus-3-",
		"claude-haiku-3-",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(model, prefix) {
			// Extract just the minor version number (e.g. "5" from "claude-sonnet-4-5-20250929")
			rest := model[len(prefix):]
			minor := strings.SplitN(rest, "-", 2)[0]
			// Rebuild with dot: "claude-sonnet-4.5"
			base := prefix[:len(prefix)-1] // strip trailing "-"
			return base + "." + minor
		}
	}
	return model
}

// buildOpenAIMessages converts the Anthropic messages array (plus optional system
// prompt) into the OpenAI messages array.
func buildOpenAIMessages(req AnthropicMessagesRequest) []openAIMessage {
	var msgs []openAIMessage

	// System prompt
	if len(req.System) > 0 && string(req.System) != "null" {
		systemText := extractSystemText(req.System)
		if systemText != "" {
			msgs = append(msgs, openAIMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// Conversation messages
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			msgs = append(msgs, handleAnthropicUserMessage(m)...)
		case "assistant":
			msgs = append(msgs, handleAnthropicAssistantMessage(m))
		}
	}

	return msgs
}

// extractSystemText parses the Anthropic system field, which can be either a
// plain string or an array of text blocks.
func extractSystemText(raw json.RawMessage) string {
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try array of text blocks.
	var blocks []AnthropicTextBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n\n")
	}

	return ""
}

// handleAnthropicUserMessage converts an Anthropic user message to zero or more
// OpenAI messages.  tool_result blocks become separate "tool" role messages.
func handleAnthropicUserMessage(m AnthropicMessage) []openAIMessage {
	// Attempt to decode as plain string.
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		return []openAIMessage{{Role: "user", Content: text}}
	}

	// Decode as array of blocks.
	var blocks []json.RawMessage
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return []openAIMessage{{Role: "user", Content: string(m.Content)}}
	}

	var toolResults []openAIMessage
	var otherParts []openAIContentPart

	for _, raw := range blocks {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}

		switch typed.Type {
		case "tool_result":
			var tr AnthropicToolResultBlock
			if err := json.Unmarshal(raw, &tr); err == nil {
				toolResults = append(toolResults, openAIMessage{
					Role:       "tool",
					Content:    tr.Content,
					ToolCallID: tr.ToolUseID,
				})
			}
		case "text":
			var tb AnthropicTextBlock
			if err := json.Unmarshal(raw, &tb); err == nil && tb.Text != "" {
				otherParts = append(otherParts, openAIContentPart{
					Type: "text",
					Text: tb.Text,
				})
			}
		case "image":
			var ib AnthropicImageBlock
			if err := json.Unmarshal(raw, &ib); err == nil {
				otherParts = append(otherParts, openAIContentPart{
					Type: "image_url",
					ImageURL: &openAIImageURLObject{
						URL: fmt.Sprintf("data:%s;base64,%s", ib.Source.MediaType, ib.Source.Data),
					},
				})
			}
		}
	}

	// tool_result messages must come first (protocol: tool_use → tool_result → next_user).
	var result []openAIMessage
	result = append(result, toolResults...)

	if len(otherParts) > 0 {
		if len(otherParts) == 1 && otherParts[0].Type == "text" {
			result = append(result, openAIMessage{Role: "user", Content: otherParts[0].Text})
		} else {
			result = append(result, openAIMessage{Role: "user", Content: otherParts})
		}
	}

	if len(result) == 0 {
		result = append(result, openAIMessage{Role: "user", Content: ""})
	}

	return result
}

// handleAnthropicAssistantMessage converts an Anthropic assistant message to a
// single OpenAI assistant message (with optional tool_calls).
func handleAnthropicAssistantMessage(m AnthropicMessage) openAIMessage {
	// Plain string content.
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		return openAIMessage{Role: "assistant", Content: text}
	}

	// Array of blocks.
	var blocks []json.RawMessage
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return openAIMessage{Role: "assistant", Content: string(m.Content)}
	}

	var textParts []string
	var toolCalls []openAIToolCall

	for _, raw := range blocks {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}

		switch typed.Type {
		case "text":
			var tb AnthropicTextBlock
			if err := json.Unmarshal(raw, &tb); err == nil {
				textParts = append(textParts, tb.Text)
			}
		case "thinking":
			var thk AnthropicThinkingBlock
			if err := json.Unmarshal(raw, &thk); err == nil {
				textParts = append(textParts, thk.Thinking)
			}
		case "tool_use":
			var tu AnthropicToolUseBlock
			if err := json.Unmarshal(raw, &tu); err == nil {
				argBytes, _ := json.Marshal(tu.Input)
				toolCalls = append(toolCalls, openAIToolCall{
					ID:   tu.ID,
					Type: "function",
					Function: openAIFunctionCall{
						Name:      tu.Name,
						Arguments: string(argBytes),
					},
				})
			}
		}
	}

	combined := strings.Join(textParts, "\n\n")
	msg := openAIMessage{Role: "assistant"}
	if combined != "" {
		msg.Content = combined
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	return msg
}

// translateTools converts Anthropic tool definitions to OpenAI format.
func translateTools(tools []AnthropicTool) []openAITool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		result = append(result, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return result
}

// translateToolChoice converts Anthropic tool_choice to OpenAI format.
func translateToolChoice(tc *AnthropicToolChoice) any {
	if tc == nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		if tc.Name != "" {
			return map[string]any{
				"type":     "function",
				"function": map[string]string{"name": tc.Name},
			}
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Response translation: OpenAI → Anthropic
// ─────────────────────────────────────────────────────────────────────────────

// translateOpenAIToAnthropic converts a non-streaming OpenAI response body to
// an Anthropic response body.
func translateOpenAIToAnthropic(body []byte) ([]byte, error) {
	var resp openAIChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openai response: %w", err)
	}

	var contentBlocks []json.RawMessage
	var stopReason string

	for i, choice := range resp.Choices {
		if i == 0 {
			stopReason = mapOpenAIFinishReasonToAnthropic(choice.FinishReason)
		}

		// Text content
		if text, ok := choice.Message.Content.(string); ok && text != "" {
			block, _ := json.Marshal(AnthropicTextBlock{Type: "text", Text: text})
			contentBlocks = append(contentBlocks, block)
		}

		// Tool use blocks
		for _, tc := range choice.Message.ToolCalls {
			var input json.RawMessage
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			if input == nil {
				input = json.RawMessage("{}")
			}
			block, _ := json.Marshal(AnthropicToolUseBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
			contentBlocks = append(contentBlocks, block)
			if stopReason == "end_turn" {
				stopReason = "tool_use"
			}
		}
	}

	if len(contentBlocks) == 0 {
		empty, _ := json.Marshal(AnthropicTextBlock{Type: "text", Text: ""})
		contentBlocks = append(contentBlocks, empty)
	}

	anthropicResp := AnthropicMessagesResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      resp.Model,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage:      buildAnthropicUsage(resp.Usage),
	}

	return json.Marshal(anthropicResp)
}

// buildAnthropicUsage converts OpenAI usage to Anthropic usage.
func buildAnthropicUsage(u *openAIUsage) AnthropicUsage {
	if u == nil {
		return AnthropicUsage{}
	}
	au := AnthropicUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptDetails != nil {
		au.CacheReadInputTokens = u.PromptDetails.CachedTokens
		au.InputTokens = u.PromptTokens - u.PromptDetails.CachedTokens
	}
	return au
}

// mapOpenAIFinishReasonToAnthropic maps OpenAI finish_reason to Anthropic stop_reason.
func mapOpenAIFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Streaming state machine: OpenAI chunks → Anthropic SSE events
// ─────────────────────────────────────────────────────────────────────────────

// copilotStreamState tracks the state of an in-progress stream translation.
type copilotStreamState struct {
	messageStartSent bool
	blockIndex       int
	blockOpen        bool
	// toolCalls maps OpenAI tool_call index → Anthropic block index + metadata
	toolCalls map[int]copilotToolCallInfo
}

type copilotToolCallInfo struct {
	id                 string
	name               string
	anthropicBlockIdx  int
}

// translateChunkToAnthropicEvents converts a single OpenAI SSE chunk to zero or
// more Anthropic SSE event payloads (each is a JSON string to be written as
// "data: <json>\n\n").
func translateChunkToAnthropicEvents(
	chunk *openAIChatStreamChunk,
	state *copilotStreamState,
) []string {
	var events []string
	if len(chunk.Choices) == 0 {
		return events
	}
	choice := chunk.Choices[0]
	delta := choice.Delta

	// Send message_start once per stream.
	if !state.messageStartSent {
		inputTokens := 0
		cacheRead := 0
		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
			if chunk.Usage.PromptDetails != nil {
				cacheRead = chunk.Usage.PromptDetails.CachedTokens
				inputTokens -= cacheRead
			}
		}
		evt := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            chunk.ID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         chunk.Model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":           inputTokens,
					"output_tokens":          0,
					"cache_read_input_tokens": cacheRead,
				},
			},
		}
		if b, err := json.Marshal(evt); err == nil {
			events = append(events, string(b))
		}
		state.messageStartSent = true

		// Anthropic expects a leading ping event.
		pingJSON, _ := json.Marshal(map[string]string{"type": "ping"})
		events = append(events, string(pingJSON))
	}

	// Text delta content.
	if delta.Content != "" {
		if isToolBlockOpen(state) {
			// Close the open tool block before starting text.
			events = append(events, blockStopEvent(state.blockIndex))
			state.blockIndex++
			state.blockOpen = false
		}
		if !state.blockOpen {
			events = append(events, contentBlockStart(state.blockIndex, "text"))
			state.blockOpen = true
		}
		events = append(events, textDeltaEvent(state.blockIndex, delta.Content))
	}

	// Tool call deltas.
	for _, tc := range delta.ToolCalls {
		if tc.ID != "" && tc.Function.Name != "" {
			// New tool call starting.
			if state.blockOpen {
				events = append(events, blockStopEvent(state.blockIndex))
				state.blockIndex++
				state.blockOpen = false
			}
			anthropicIdx := state.blockIndex
			state.toolCalls[tc.Index] = copilotToolCallInfo{
				id:                tc.ID,
				name:              tc.Function.Name,
				anthropicBlockIdx: anthropicIdx,
			}
			events = append(events, toolUseBlockStart(anthropicIdx, tc.ID, tc.Function.Name))
			state.blockOpen = true
		}
		if tc.Function.Arguments != "" {
			if info, ok := state.toolCalls[tc.Index]; ok {
				events = append(events, inputJSONDeltaEvent(info.anthropicBlockIdx, tc.Function.Arguments))
			}
		}
	}

	// Stream finished.
	if choice.FinishReason != "" {
		if state.blockOpen {
			events = append(events, blockStopEvent(state.blockIndex))
			state.blockOpen = false
		}

		outputTokens := 0
		inputTokens := 0
		if chunk.Usage != nil {
			outputTokens = chunk.Usage.CompletionTokens
			inputTokens = chunk.Usage.PromptTokens
			if chunk.Usage.PromptDetails != nil {
				inputTokens -= chunk.Usage.PromptDetails.CachedTokens
			}
		}

		stopReason := mapOpenAIFinishReasonToAnthropic(choice.FinishReason)
		msgDelta := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   stopReason,
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
		}
		if b, err := json.Marshal(msgDelta); err == nil {
			events = append(events, string(b))
		}

		msgStop, _ := json.Marshal(map[string]string{"type": "message_stop"})
		events = append(events, string(msgStop))
	}

	return events
}

// isToolBlockOpen returns true if the currently open block is a tool_use block.
func isToolBlockOpen(state *copilotStreamState) bool {
	if !state.blockOpen {
		return false
	}
	for _, info := range state.toolCalls {
		if info.anthropicBlockIdx == state.blockIndex {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// SSE event constructors
// ─────────────────────────────────────────────────────────────────────────────

func blockStopEvent(idx int) string {
	b, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": idx})
	return string(b)
}

func contentBlockStart(idx int, blockType string) string {
	b, _ := json.Marshal(map[string]any{
		"type":  "content_block_start",
		"index": idx,
		"content_block": map[string]any{
			"type": blockType,
			"text": "",
		},
	})
	return string(b)
}

func toolUseBlockStart(idx int, id, name string) string {
	b, _ := json.Marshal(map[string]any{
		"type":  "content_block_start",
		"index": idx,
		"content_block": map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": map[string]any{},
		},
	})
	return string(b)
}

func textDeltaEvent(idx int, text string) string {
	b, _ := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": idx,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	})
	return string(b)
}

func inputJSONDeltaEvent(idx int, partial string) string {
	b, _ := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": idx,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": partial,
		},
	})
	return string(b)
}
