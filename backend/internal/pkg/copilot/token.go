package copilot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ExchangeToken exchanges a GitHub personal access token for a short-lived Copilot API token.
//
// The returned CopilotToken typically expires in ~30 minutes. Callers should use
// RefreshIn to schedule proactive refresh.
func ExchangeToken(httpClient *http.Client, githubToken string) (*CopilotToken, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest(http.MethodGet, TokenExchangeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: build request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("editor-version", DefaultEditorVersion)
	req.Header.Set("editor-plugin-version", DefaultEditorPluginVersion)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("x-github-api-version", DefaultGitHubAPIVersion)
	req.Header.Set("x-vscode-user-agent-library-version", "electron-fetch")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot token exchange: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenExchangeResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("copilot token exchange: parse response: %w", err)
	}

	if tokenResp.Token == "" {
		errMsg := tokenResp.ErrorMessage
		if errMsg == "" {
			errMsg = "empty token in response"
		}
		return nil, fmt.Errorf("copilot token exchange: %s", errMsg)
	}

	now := time.Now()
	expiresAt := now.Add(30 * time.Minute) // default fallback
	if tokenResp.ExpiresAt > 0 {
		expiresAt = time.Unix(tokenResp.ExpiresAt, 0)
	}

	// Proactively refresh 60 seconds before the token's own refresh_in hint,
	// or 60 seconds before expiry if refresh_in is absent.
	refreshIn := tokenResp.RefreshIn
	if refreshIn <= 0 {
		refreshIn = int64(time.Until(expiresAt).Seconds()) - 60
	}
	if refreshIn < 30 {
		refreshIn = 30 // minimum 30 seconds
	}
	refreshAt := now.Add(time.Duration(refreshIn-60) * time.Second)

	return &CopilotToken{
		Token:     tokenResp.Token,
		ExpiresAt: expiresAt,
		RefreshAt: refreshAt,
	}, nil
}
