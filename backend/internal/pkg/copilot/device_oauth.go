package copilot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestDeviceCode initiates the GitHub device code flow.
func RequestDeviceCode(httpClient *http.Client) (*DeviceCodeResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"client_id": {DeviceOAuthClientID},
		"scope":     {"read:user"},
	}

	req, err := http.NewRequest(http.MethodPost, DeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("device code request: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("device code request: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("device code request: parse response: %w", err)
	}

	return &result, nil
}

// PollAccessToken polls GitHub for the access token using the device code.
// Returns the access token response, or an error.
// The caller should handle "authorization_pending" and "slow_down" errors by retrying.
func PollAccessToken(httpClient *http.Client, deviceCode string) (*AccessTokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"client_id":   {DeviceOAuthClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequest(http.MethodPost, AccessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("poll access token: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll access token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("poll access token: read body: %w", err)
	}

	var result AccessTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("poll access token: parse response: %w (body=%s)", err, string(body))
	}

	return &result, nil
}

// GetGitHubUser fetches the authenticated user's profile.
func GetGitHubUser(httpClient *http.Client, accessToken string) (*GitHubUser, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest(http.MethodGet, GitHubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get github user: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get github user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get github user: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get github user: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var user GitHubUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("get github user: parse response: %w", err)
	}

	return &user, nil
}
