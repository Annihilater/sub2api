// Package copilot provides types and utilities for the GitHub Copilot API.
package copilot

import "time"

// TokenExchangeResponse is the response from the Copilot token exchange endpoint.
// Endpoint: GET https://api.github.com/copilot_internal/v2/token
type TokenExchangeResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	RefreshIn int64  `json:"refresh_in"`
	// ErrorMessage is populated when the exchange fails (e.g., no Copilot subscription).
	ErrorMessage string `json:"error_description,omitempty"`
}

// CopilotToken holds a cached Copilot API token with its refresh metadata.
type CopilotToken struct {
	// Token is the Bearer token for Copilot API requests.
	Token string
	// ExpiresAt is the token expiration time.
	ExpiresAt time.Time
	// RefreshAt is when the token should be proactively refreshed.
	RefreshAt time.Time
}

// IsExpired reports whether the token has expired (with 60s safety margin).
func (t *CopilotToken) IsExpired() bool {
	return time.Now().Add(60 * time.Second).After(t.ExpiresAt)
}

// ShouldRefresh reports whether the token should be refreshed.
func (t *CopilotToken) ShouldRefresh() bool {
	return time.Now().After(t.RefreshAt)
}

// CopilotAPIBase is the default base URL for the Copilot API (individual accounts).
// Individual accounts use api.githubcopilot.com (no subdomain prefix).
// Business/enterprise accounts use api.business.githubcopilot.com etc.
const CopilotAPIBase = "https://api.githubcopilot.com"

// TokenExchangeURL is the GitHub endpoint for exchanging a GitHub token for a Copilot token.
const TokenExchangeURL = "https://api.github.com/copilot_internal/v2/token"

// DefaultEditorVersion is the editor version header sent to the Copilot API.
const DefaultEditorVersion = "vscode/1.98.1"

// DefaultEditorPluginVersion is the editor plugin version header sent to the Copilot API.
const DefaultEditorPluginVersion = "copilot-chat/0.26.7"

// DefaultUserAgent is the user agent string sent to the Copilot API.
const DefaultUserAgent = "GitHubCopilotChat/0.26.7"

// DefaultGitHubAPIVersion is the GitHub API version header.
const DefaultGitHubAPIVersion = "2025-04-01"

// DefaultCopilotIntegrationID is the integration identifier sent to the Copilot API.
const DefaultCopilotIntegrationID = "vscode-chat"

// DefaultOpenAIIntent is the OpenAI intent header sent to the Copilot API.
const DefaultOpenAIIntent = "conversation-panel"

// DefaultTestModel is the default model used for Copilot account testing.
const DefaultTestModel = "gpt-4o"

// GitHub Device OAuth constants (VS Code's public client ID)
const (
	DeviceOAuthClientID = "Iv1.b507a08c87ecfe98"
	DeviceCodeURL       = "https://github.com/login/device/code"
	AccessTokenURL      = "https://github.com/login/oauth/access_token"
	GitHubUserURL       = "https://api.github.com/user"
)

// DeviceCodeResponse is the response from GitHub's device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// AccessTokenResponse is the response from GitHub's access token endpoint.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
	// Interval is only present in slow_down responses; callers must honor it.
	Interval int `json:"interval,omitempty"`
}

// GitHubUser is a minimal GitHub user profile.
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
}

// Model represents a Copilot model, using the same format as other platforms.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// DefaultModels is the list of models commonly available via Copilot.
// These are the well-known models from the GitHub Copilot API.
// Claude model IDs use dash-separated format (e.g. "claude-sonnet-4-5") so that
// Claude Code's built-in model whitelist accepts them. The Copilot API itself uses
// dot-separated format (e.g. "claude-sonnet-4.5") which is applied automatically
// by normalizeCopilotModel when forwarding requests.
var DefaultModels = []Model{
	{ID: "gpt-4o", Object: "model", Type: "model", DisplayName: "GPT-4o"},
	{ID: "gpt-4o-mini", Object: "model", Type: "model", DisplayName: "GPT-4o Mini"},
	{ID: "gpt-4.1", Object: "model", Type: "model", DisplayName: "GPT-4.1"},
	{ID: "gpt-4.1-mini", Object: "model", Type: "model", DisplayName: "GPT-4.1 Mini"},
	{ID: "gpt-4.1-nano", Object: "model", Type: "model", DisplayName: "GPT-4.1 Nano"},
	{ID: "o4-mini", Object: "model", Type: "model", DisplayName: "o4 Mini"},
	{ID: "o3-mini", Object: "model", Type: "model", DisplayName: "o3 Mini"},
	{ID: "claude-sonnet-4", Object: "model", Type: "model", DisplayName: "Claude Sonnet 4"},
	{ID: "claude-sonnet-4-5", Object: "model", Type: "model", DisplayName: "Claude Sonnet 4.5"},
	{ID: "claude-sonnet-4-6", Object: "model", Type: "model", DisplayName: "Claude Sonnet 4.6"},
	{ID: "claude-opus-4-5", Object: "model", Type: "model", DisplayName: "Claude Opus 4.5"},
	{ID: "claude-opus-4-6", Object: "model", Type: "model", DisplayName: "Claude Opus 4.6"},
	{ID: "claude-haiku-4-5", Object: "model", Type: "model", DisplayName: "Claude Haiku 4.5"},
	{ID: "claude-3.5-sonnet", Object: "model", Type: "model", DisplayName: "Claude 3.5 Sonnet"},
	{ID: "gemini-2.0-flash-001", Object: "model", Type: "model", DisplayName: "Gemini 2.0 Flash"},
}

// QuotaDetail holds usage information for a single Copilot feature.
type QuotaDetail struct {
	// Entitlement is the total allowed quota (-1 or absent means unlimited).
	Entitlement int `json:"entitlement,omitempty"`
	// OveragePermitted indicates whether overage beyond the entitlement is allowed.
	OveragePermitted bool `json:"overage_permitted,omitempty"`
	// Used is the number of quota units consumed so far.
	Used int `json:"used,omitempty"`
}

// CopilotQuotaInfo holds the quota and plan information for a Copilot account.
// This is derived from the GitHub API endpoint:
// GET https://api.github.com/copilot_internal/user
type CopilotQuotaInfo struct {
	// Plan is the Copilot plan type, e.g. "copilot_enterprise", "copilot_business", "copilot_for_individuals".
	Plan string `json:"plan,omitempty"`
	// PlanType is a human-readable plan label, e.g. "Individual", "Business".
	PlanType string `json:"plan_type,omitempty"`
	// SKU is the subscription SKU string.
	SKU string `json:"sku,omitempty"`
	// Chat holds chat quota details.
	Chat *QuotaDetail `json:"chat,omitempty"`
	// Completions holds code completion quota details.
	Completions *QuotaDetail `json:"completions,omitempty"`
	// PremiumInteractions holds premium interaction quota details.
	PremiumInteractions *QuotaDetail `json:"premium_interactions,omitempty"`
	// QuotaResetDate is the ISO-8601 date when the quota resets (e.g. "2026-04-01").
	QuotaResetDate string `json:"quota_reset_date,omitempty"`
}
