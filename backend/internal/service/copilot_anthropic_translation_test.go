package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// translateAnthropicToOpenAIHelper is a thin wrapper for test convenience.
func mustTranslate(t *testing.T, anthropicJSON string) map[string]any {
	t.Helper()
	out, err := translateAnthropicToOpenAI([]byte(anthropicJSON), nil)
	if err != nil {
		t.Fatalf("translateAnthropicToOpenAI error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal translated body: %v", err)
	}
	return m
}

func messagesFrom(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["messages"]
	if !ok {
		t.Fatal("no messages field")
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("messages is not array: %T", raw)
	}
	result := make([]map[string]any, len(arr))
	for i, v := range arr {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("messages[%d] is not map[string]any: %T", i, v)
		}
		result[i] = m
	}
	return result
}

// ── model passthrough ─────────────────────────────────────────────────────────

func TestTranslateAnthropicToOpenAI_ModelPassthrough(t *testing.T) {
	// Model name must be passed through exactly — no normalization.
	// The Copilot API returns model IDs with version suffixes (e.g. "claude-sonnet-4.6"),
	// so the exact ID from /models must reach the upstream unchanged.
	tests := []struct {
		model string
	}{
		{"claude-sonnet-4.6"},
		{"claude-sonnet-4.5"},
		{"claude-opus-4.6"},
		{"claude-haiku-4.5"},
		{"gpt-4o"},
		{"claude-sonnet-4"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			body := `{"model":"` + tt.model + `","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
			m := mustTranslate(t, body)
			if got := m["model"]; got != tt.model {
				t.Errorf("model = %q, want %q (no transformation expected)", got, tt.model)
			}
		})
	}
}

// ── user message content ──────────────────────────────────────────────────────

func TestTranslateAnthropicToOpenAI_ConsecutiveUserMessagesMerged(t *testing.T) {
	// Consecutive user messages are merged into one to satisfy Copilot API.
	// CC sends consecutive user messages (e.g. deferred-tools injection + real message).
	body := `{
		"model": "claude-sonnet-4.6",
		"max_tokens": 1024,
		"stream": true,
		"messages": [
			{"role": "user", "content": "<available-deferred-tools>\nAgent\n</available-deferred-tools>"},
			{"role": "user", "content": [{"type": "text", "text": "hello world"}]}
		]
	}`
	msgs := messagesFrom(t, mustTranslate(t, body))

	// Should be exactly 1 merged user message
	userMsgs := 0
	for _, m := range msgs {
		if m["role"] == "user" {
			userMsgs++
		}
	}
	if userMsgs != 1 {
		t.Errorf("expected 1 merged user message, got %d: %v", userMsgs, msgs)
	}

	// Merged content should contain both parts
	for _, m := range msgs {
		if m["role"] == "user" {
			content, ok := m["content"].(string)
			if !ok {
				t.Fatalf("merged content should be string, got %T", m["content"])
			}
			if !strings.Contains(content, "available-deferred-tools") {
				t.Error("merged content missing first message text")
			}
			if !strings.Contains(content, "hello world") {
				t.Error("merged content missing second message text")
			}
		}
	}
}

// ── sanitize: orphan tool_calls and empty user messages ─────────────────────

func TestTranslateAnthropicToOpenAI_OrphanToolCallGetsSyntheticResponse(t *testing.T) {
	// When assistant has a tool_call but the user message is empty (no tool_result),
	// the sanitizer must inject a synthetic tool response so Copilot API accepts it.
	body := `{
		"model": "claude-sonnet-4.6",
		"max_tokens": 1024,
		"stream": true,
		"messages": [
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "t1", "name": "ToolSearch", "input": {"query": "test"}}]},
			{"role": "user", "content": ""},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "t2", "name": "Bash", "input": {"command": "ls"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "t2", "content": "file.txt"}]},
			{"role": "assistant", "content": "done"}
		]
	}`
	msgs := messagesFrom(t, mustTranslate(t, body))

	// No empty user messages should remain
	for i, m := range msgs {
		if m["role"] == "user" {
			content, ok := m["content"].(string)
			if ok && content == "" {
				t.Errorf("message [%d] is an empty user message — should have been removed", i)
			}
		}
	}

	// The orphan tool_call t1 (ToolSearch) should have a synthetic tool response
	foundT1Response := false
	for _, m := range msgs {
		if m["role"] == "tool" && m["tool_call_id"] == "t1" {
			foundT1Response = true
		}
	}
	if !foundT1Response {
		t.Error("orphan tool_call t1 should have a synthetic tool response injected")
	}

	// No consecutive same-role messages should exist
	for i := 1; i < len(msgs); i++ {
		if msgs[i]["role"] == msgs[i-1]["role"] {
			t.Errorf("consecutive same-role at [%d]-[%d]: both %q", i-1, i, msgs[i]["role"])
		}
	}
}

func TestTranslateAnthropicToOpenAI_AlternatingRolesNotMerged(t *testing.T) {
	// Normal alternating user/assistant must NOT be merged
	body := `{
		"model": "claude-sonnet-4.6",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "hello"},
			{"role": "user", "content": "how are you?"}
		]
	}`
	msgs := messagesFrom(t, mustTranslate(t, body))

	// Should have 3 messages (no system), roles: user, assistant, user
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	roles := make([]string, 3)
	for i := range roles {
		r, ok := msgs[i]["role"].(string)
		if !ok {
			t.Fatalf("msg[%d].role is not string: %T", i, msgs[i]["role"])
		}
		roles[i] = r
	}
	want := []string{"user", "assistant", "user"}
	for i, r := range roles {
		if r != want[i] {
			t.Errorf("msg[%d].role = %q, want %q", i, r, want[i])
		}
	}
}

func TestTranslateAnthropicToOpenAI_UserMessage_PlainString(t *testing.T) {
	body := `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hello"}],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	if len(msgs) != 1 || msgs[0]["role"] != "user" {
		t.Fatalf("unexpected messages: %v", msgs)
	}
	if msgs[0]["content"] != "hello" {
		t.Errorf("content = %v, want 'hello'", msgs[0]["content"])
	}
}

func TestTranslateAnthropicToOpenAI_UserMessage_MultiTextJoined(t *testing.T) {
	// Multiple text blocks → joined plain string (matches TypeScript mapContent no-image path)
	body := `{"model":"claude-sonnet-4","messages":[{"role":"user","content":[
		{"type":"text","text":"first"},
		{"type":"text","text":"second"}
	]}],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	// system none, so index 0 is the user message
	userMsg := msgs[0]
	got, ok := userMsg["content"].(string)
	if !ok {
		t.Fatalf("content should be string, got %T: %v", userMsg["content"], userMsg["content"])
	}
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("content = %q, want both 'first' and 'second'", got)
	}
}

func TestTranslateAnthropicToOpenAI_UserMessage_WithImage_IsArray(t *testing.T) {
	// Image present → content-parts array
	body := `{"model":"claude-sonnet-4","messages":[{"role":"user","content":[
		{"type":"text","text":"look at this"},
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc="}}
	]}],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	userMsg := msgs[0]
	arr, ok := userMsg["content"].([]any)
	if !ok {
		t.Fatalf("content should be array, got %T", userMsg["content"])
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 content parts, got %d", len(arr))
	}
}

func TestTranslateAnthropicToOpenAI_ToolResult_BecomesToolRole(t *testing.T) {
	body := `{"model":"claude-sonnet-4","messages":[
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"call_1","content":"42"},
			{"type":"text","text":"done"}
		]}
	],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	if msgs[0]["role"] != "tool" {
		t.Errorf("first msg role = %v, want 'tool'", msgs[0]["role"])
	}
	if msgs[0]["tool_call_id"] != "call_1" {
		t.Errorf("tool_call_id = %v, want 'call_1'", msgs[0]["tool_call_id"])
	}
	if msgs[1]["role"] != "user" {
		t.Errorf("second msg role = %v, want 'user'", msgs[1]["role"])
	}
}

// ── assistant message content ─────────────────────────────────────────────────

func TestTranslateAnthropicToOpenAI_AssistantMessage_ThinkingMerged(t *testing.T) {
	// thinking block should be merged with text in assistant message
	body := `{"model":"claude-sonnet-4","messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"let me think"},
			{"type":"text","text":"answer"}
		]}
	],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	// [system?] user, assistant — find assistant
	var assistantMsg map[string]any
	for _, m := range msgs {
		if m["role"] == "assistant" {
			assistantMsg = m
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("no assistant message found")
	}
	content, ok := assistantMsg["content"].(string)
	if !ok {
		t.Fatalf("assistant content should be string, got %T", assistantMsg["content"])
	}
	if !strings.Contains(content, "let me think") {
		t.Errorf("thinking not merged into content: %q", content)
	}
	if !strings.Contains(content, "answer") {
		t.Errorf("text not in content: %q", content)
	}
}

func TestTranslateAnthropicToOpenAI_AssistantMessage_ToolUse_ContentNull(t *testing.T) {
	// When assistant has only tool_use blocks (no text), content should be null
	body := `{"model":"claude-sonnet-4","messages":[
		{"role":"user","content":"call tool"},
		{"role":"assistant","content":[
			{"type":"tool_use","id":"call_1","name":"my_tool","input":{"x":1}}
		]}
	],"max_tokens":10}`
	msgs := messagesFrom(t, mustTranslate(t, body))
	var assistantMsg map[string]any
	for _, m := range msgs {
		if m["role"] == "assistant" {
			assistantMsg = m
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("no assistant message found")
	}
	// content should be nil/null (not set)
	if v, exists := assistantMsg["content"]; exists && v != nil {
		t.Errorf("content should be null when only tool_use blocks, got %v", v)
	}
	// tool_calls should be present
	tcs, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(tcs) == 0 {
		t.Errorf("tool_calls not found or empty: %v", assistantMsg["tool_calls"])
	}
}

// ── sanitize: real-world 400 reproduction ────────────────────────────────────

func TestTranslateAnthropicToOpenAI_Sanitize_RealWorld400Pattern(t *testing.T) {
	// Reproduces the exact message pattern from the 400 Bad Request in production:
	//   user, user, assistant, user, assistant, user, assistant(tool_use),
	//   user(""), assistant(2 tool_uses), user(2 tool_results), assistant
	//
	// Problems: orphan tool_call, empty user, consecutive user/user, consecutive tool/tool
	body := `{
		"model": "claude-sonnet-4.6",
		"max_tokens": 1024,
		"stream": true,
		"messages": [
			{"role": "user", "content": "<deferred-tools>Agent</deferred-tools>"},
			{"role": "user", "content": [{"type": "text", "text": "summarize the project"}]},
			{"role": "assistant", "content": "hello"},
			{"role": "user", "content": "what model are you?"},
			{"role": "assistant", "content": "I am Claude"},
			{"role": "user", "content": "summarize project"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Let me read the files."},
				{"type": "tool_use", "id": "t_search", "name": "ToolSearch", "input": {"query": "test"}}
			]},
			{"role": "user", "content": ""},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Reading files now."},
				{"type": "tool_use", "id": "t_glob1", "name": "Glob", "input": {"pattern": "*.md"}},
				{"type": "tool_use", "id": "t_glob2", "name": "Glob", "input": {"pattern": "*.toml"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "t_glob1", "content": "README.md"},
				{"type": "tool_result", "tool_use_id": "t_glob2", "content": "Cargo.toml"}
			]},
			{"role": "assistant", "content": "Here is the summary."}
		]
	}`
	msgs := messagesFrom(t, mustTranslate(t, body))

	// Verify: no consecutive same-role messages (tool-tool after same assistant is OK per OpenAI spec)
	for i := 1; i < len(msgs); i++ {
		r1, _ := msgs[i-1]["role"].(string)
		r2, _ := msgs[i]["role"].(string)
		if r1 == r2 && r1 != "tool" { // consecutive tool msgs are valid (multi-tool_call responses)
			t.Errorf("consecutive same-role at [%d]-[%d]: both %q", i-1, i, r1)
		}
	}

	// Verify: no empty user messages
	for i, m := range msgs {
		if m["role"] == "user" {
			if s, ok := m["content"].(string); ok && s == "" {
				t.Errorf("message [%d] is empty user message — should have been removed", i)
			}
		}
	}

	// Verify: every tool_call has a matching tool response
	allToolCalls := make(map[string]bool)
	allToolResponses := make(map[string]bool)
	for _, m := range msgs {
		if tcs, ok := m["tool_calls"].([]any); ok {
			for _, tc := range tcs {
				callMap, _ := tc.(map[string]any)
				if id, ok := callMap["id"].(string); ok {
					allToolCalls[id] = true
				}
			}
		}
		if m["role"] == "tool" {
			if id, ok := m["tool_call_id"].(string); ok {
				allToolResponses[id] = true
			}
		}
	}
	for id := range allToolCalls {
		if !allToolResponses[id] {
			t.Errorf("tool_call %q has no matching tool response", id)
		}
	}

	// Specifically check: t_search (orphan) should have a synthetic tool response
	if !allToolResponses["t_search"] {
		t.Error("orphan tool_call t_search should have a synthetic tool response")
	}

	// Debug: print message structure
	t.Logf("sanitized message count: %d", len(msgs))
	for i, m := range msgs {
		role, _ := m["role"].(string)
		hasTC := m["tool_calls"] != nil
		hasTCID := m["tool_call_id"] != nil
		t.Logf("  [%d] role=%s has_tool_calls=%v has_tool_call_id=%v", i, role, hasTC, hasTCID)
	}
}
