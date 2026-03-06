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
	Model         string               `json:"model"`
	Messages      []AnthropicMessage   `json:"messages"`
	MaxTokens     int                  `json:"max_tokens"`
	System        json.RawMessage      `json:"system,omitempty"` // string or []AnthropicTextBlock
	Metadata      *AnthropicMetadata   `json:"metadata,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	Tools         []AnthropicTool      `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice `json:"tool_choice,omitempty"`
}

type AnthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// AnthropicMessage is a single turn in the conversation.
type AnthropicMessage struct {
	Role    string          `json:"role"`    // "user" | "assistant"
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
	Type      string `json:"type"` // "tool_result"
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
	ID           string            `json:"id"`
	Type         string            `json:"type"` // "message"
	Role         string            `json:"role"` // "assistant"
	Model        string            `json:"model"`
	Content      []json.RawMessage `json:"content"` // []AnthropicTextBlock | []AnthropicToolUseBlock
	StopReason   string            `json:"stop_reason"`
	StopSequence *string           `json:"stop_sequence"`
	Usage        AnthropicUsage    `json:"usage"`
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
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	User        string          `json:"user,omitempty"`
	Tools       []openAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
}

// openAIMessage is a single message in an OpenAI chat request.
type openAIMessage struct {
	Role       string           `json:"role"`    // system | user | assistant | tool
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
	Type     string         `json:"type"` // "function"
	Function openAIFunction `json:"function"`
}

// openAIFunction describes a callable function.
type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// openAIToolCall is a tool call made by the model.
type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"` // "function"
	Function openAIFunctionCall `json:"function"`
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
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage,omitempty"`
}

// openAIChoice is a single completion choice.
type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
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
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage         `json:"usage,omitempty"`
}

// openAIStreamChoice is a choice delta in a streaming chunk.
type openAIStreamChoice struct {
	Index        int         `json:"index"`
	Delta        openAIDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

// openAIDelta is the incremental content in a stream chunk.
type openAIDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   string                `json:"content,omitempty"`
	ToolCalls []openAIToolCallDelta `json:"tool_calls,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Request translation: Anthropic → OpenAI
// ─────────────────────────────────────────────────────────────────────────────

// translateAnthropicToOpenAI converts an Anthropic /v1/messages request body to
// an OpenAI /chat/completions request body suitable for the Copilot API.
//
// The model name is passed through exactly as provided by the client.
// The Copilot API returns model IDs with version suffixes (e.g. "claude-sonnet-4.6"),
// so clients should use the exact ID from the /models endpoint.
func translateAnthropicToOpenAI(body []byte, _ map[string]string) ([]byte, error) {
	var req AnthropicMessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}

	openAIReq := openAIChatRequest{
		Model:       req.Model,
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

	return sanitizeOpenAIMessages(msgs)
}

// sanitizeOpenAIMessages normalizes translated messages to satisfy Copilot API
// constraints that are stricter than the Anthropic protocol:
//
//  1. Every assistant tool_call must have a matching tool-role response.
//     Claude Code sometimes sends an empty user acknowledgement instead of a
//     tool_result (e.g. after ToolSearch deferred-tool lookup), which leaves
//     the tool_call "orphaned" in OpenAI format.  We inject a synthetic tool
//     response so the Copilot API accepts the conversation.
//
//  2. Consecutive same-role messages (user/user, system/system) are merged
//     because the Copilot API may reject them.
//
//  3. Empty user messages (content == "") that sit between two assistant
//     messages are removed when they serve no purpose.
func sanitizeOpenAIMessages(msgs []openAIMessage) []openAIMessage {
	// ── Step 1: inject synthetic tool responses for orphan tool_calls ────

	// Collect all tool_call IDs that already have a tool response.
	answered := make(map[string]struct{})
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID != "" {
			answered[m.ToolCallID] = struct{}{}
		}
	}

	// Walk messages; after each assistant with unanswered tool_calls, insert
	// synthetic tool responses immediately following that assistant message.
	var patched []openAIMessage
	for _, m := range msgs {
		patched = append(patched, m)
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if _, ok := answered[tc.ID]; !ok {
					patched = append(patched, openAIMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    "",
					})
					answered[tc.ID] = struct{}{}
				}
			}
		}
	}

	// ── Step 2: remove empty user messages that are now unnecessary ──────
	// An empty user message between two non-user messages serves no purpose
	// and can cause "consecutive same-role" issues once we injected synthetic
	// tool responses above.
	var cleaned []openAIMessage
	for i, m := range patched {
		if m.Role == "user" {
			if s, ok := m.Content.(string); ok && s == "" {
				// Keep if it is the only message, or the last message
				if len(patched) > 1 && i < len(patched)-1 {
					continue
				}
			}
		}
		cleaned = append(cleaned, m)
	}

	// ── Step 3: merge consecutive user/system messages ───────────────────
	if len(cleaned) == 0 {
		return cleaned
	}
	result := make([]openAIMessage, 0, len(cleaned))
	result = append(result, cleaned[0])

	for i := 1; i < len(cleaned); i++ {
		cur := cleaned[i]
		prev := &result[len(result)-1]

		canMerge := cur.Role == prev.Role &&
			(cur.Role == "user" || cur.Role == "system") &&
			len(cur.ToolCalls) == 0 &&
			len(prev.ToolCalls) == 0

		if !canMerge {
			result = append(result, cur)
			continue
		}

		prevText := contentToString(prev.Content)
		curText := contentToString(cur.Content)
		merged := prevText
		if curText != "" {
			if merged != "" {
				merged += "\n\n"
			}
			merged += curText
		}
		prev.Content = merged
	}

	return result
}

// contentToString extracts a plain string from an openAIMessage Content value.
func contentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []openAIContentPart:
		var parts []string
		for _, p := range v {
			if p.Type == "text" && p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		return ""
	}
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
	// hasImage tracks whether any image block is present in non-tool_result blocks.
	// When true we must use content-parts array; when false we join text/thinking as a plain string.
	// This matches TypeScript mapContent() semantics exactly.
	var hasImage bool
	var textParts []string          // text + thinking joined when no image
	var imageParts []openAIContentPart // full content-parts when image present

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
			if err := json.Unmarshal(raw, &tb); err == nil {
				textParts = append(textParts, tb.Text)
				imageParts = append(imageParts, openAIContentPart{Type: "text", Text: tb.Text})
			}
		case "thinking":
			// thinking blocks in user messages are rare but handled for completeness,
			// matching TypeScript mapContent() which includes block.type === "thinking".
			var thk AnthropicThinkingBlock
			if err := json.Unmarshal(raw, &thk); err == nil {
				textParts = append(textParts, thk.Thinking)
				imageParts = append(imageParts, openAIContentPart{Type: "text", Text: thk.Thinking})
			}
		case "image":
			hasImage = true
			var ib AnthropicImageBlock
			if err := json.Unmarshal(raw, &ib); err == nil {
				imageParts = append(imageParts, openAIContentPart{
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

	if len(textParts) > 0 || hasImage {
		var userContent any
		if hasImage {
			// Image present: use content-parts array (TypeScript path with hasImage=true)
			userContent = imageParts
		} else {
			// No image: join text+thinking as a plain string (TypeScript mapContent no-image path)
			userContent = strings.Join(textParts, "\n\n")
		}
		result = append(result, openAIMessage{Role: "user", Content: userContent})
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
	anthropicBlockIdx int
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
					"input_tokens":            inputTokens,
					"output_tokens":           0,
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
