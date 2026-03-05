package copilot

import (
	"net/http"

	"github.com/google/uuid"
)

// CopilotHeaders returns the standard headers required by the Copilot API.
//
// The initiator parameter controls quota consumption:
//   - "user": consumes premium request quota
//   - "agent": uses standard quota
func CopilotHeaders(initiator string, isVision bool) http.Header {
	h := http.Header{}
	h.Set("editor-version", DefaultEditorVersion)
	h.Set("editor-plugin-version", DefaultEditorPluginVersion)
	h.Set("User-Agent", DefaultUserAgent)
	h.Set("x-github-api-version", DefaultGitHubAPIVersion)
	h.Set("copilot-integration-id", DefaultCopilotIntegrationID)
	h.Set("openai-intent", DefaultOpenAIIntent)
	h.Set("x-vscode-user-agent-library-version", "electron-fetch")
	h.Set("x-request-id", uuid.New().String())

	if initiator == "" {
		initiator = "user"
	}
	h.Set("X-Initiator", initiator)

	if isVision {
		h.Set("Copilot-Vision-Request", "true")
	}

	return h
}
