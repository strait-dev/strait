package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// DeviceCodeResponse is returned by the device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceTokenResponse is returned when the device code has been approved.
type DeviceTokenResponse struct {
	APIKey    string   `json:"api_key"`
	ProjectID string   `json:"project_id"`
	Scopes    []string `json:"scopes"`
}

// RequestDeviceCode initiates the device authorization flow.
// No authentication is required for this endpoint.
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	fullURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	fullURL.Path = path.Join(fullURL.Path, "/v1/cli/auth/device-code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	return &out, nil
}

// PollDeviceToken polls the token endpoint until the device code is approved,
// expired, or the context is cancelled. It respects the polling interval and
// backs off on 429 responses.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string, interval, expiresIn int) (*DeviceTokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}

	fullURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	fullURL.Path = path.Join(fullURL.Path, "/v1/cli/auth/token")

	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	pollInterval := time.Duration(interval) * time.Second

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device code expired")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		body, err := json.Marshal(map[string]string{
			"device_code": deviceCode,
			"grant_type":  "device_code",
		})
		if err != nil {
			return nil, fmt.Errorf("marshal token request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create token request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("poll device token: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var out DeviceTokenResponse
			if decErr := json.NewDecoder(resp.Body).Decode(&out); decErr != nil {
				_ = resp.Body.Close()
				return nil, fmt.Errorf("decode token response: %w", decErr)
			}
			_ = resp.Body.Close()
			return &out, nil
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()

		// Handle 429 (rate limited) by backing off.
		if resp.StatusCode == http.StatusTooManyRequests {
			pollInterval *= 2
			continue
		}

		// Parse error response.
		var errResp struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil {
			switch errResp.Error {
			case "authorization_pending":
				continue
			case "expired_token":
				return nil, fmt.Errorf("device code expired")
			}
		}

		return nil, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}
