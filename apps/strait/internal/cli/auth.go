package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// DeviceCodeResponse is the parsed body from POST /v1/cli/auth/device-code.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse is the parsed body on a successful device token exchange.
type TokenResponse struct {
	APIKey    string   `json:"api_key"`
	ProjectID string   `json:"project_id"`
	Scopes    []string `json:"scopes"`
}

// ErrAuthorizationPending is returned while the user has not yet approved the
// device code in the dashboard.
var ErrAuthorizationPending = errors.New("authorization pending")

// ErrExpiredToken is returned when the device code has expired.
var ErrExpiredToken = errors.New("device code expired")

// ErrAlreadyExchanged is returned when the device code was already used.
var ErrAlreadyExchanged = errors.New("device code already exchanged")

// RequestDeviceCode calls POST /v1/cli/auth/device-code and returns the
// response that contains the user code and verification URL.
func RequestDeviceCode(ctx context.Context, c *Client) (*DeviceCodeResponse, error) {
	var resp DeviceCodeResponse
	if err := c.Do(ctx, "POST", "/v1/cli/auth/device-code", struct{}{}, &resp); err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	return &resp, nil
}

// PollForToken polls POST /v1/cli/auth/token until the user approves the
// request, the code expires, or ctx is cancelled. It returns the token on
// success or a typed error otherwise.
func PollForToken(ctx context.Context, c *Client, deviceCode string, interval time.Duration, deadline time.Time) (*TokenResponse, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			return nil, ErrExpiredToken
		}

		var tok TokenResponse
		err := c.Do(ctx, "POST", "/v1/cli/auth/token",
			map[string]string{
				"device_code": deviceCode,
				"grant_type":  "device_code",
			}, &tok)
		if err == nil {
			return &tok, nil
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Code {
			case "authorization_pending":
				continue
			case "expired_token":
				return nil, ErrExpiredToken
			case "token_already_exchanged":
				return nil, ErrAlreadyExchanged
			}
		}
		return nil, fmt.Errorf("poll token: %w", err)
	}
}

// OpenBrowser attempts to open url in the default system browser.
// Failures are silently ignored — the caller should always print the URL as a
// fallback so the user can open it manually.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
