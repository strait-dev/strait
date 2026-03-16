package strait

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AuthType represents the authentication strategy.
type AuthType string

const (
	AuthTypeBearer   AuthType = "bearer"
	AuthTypeAPIKey   AuthType = "apiKey"
	AuthTypeRunToken AuthType = "runToken"
)

// AuthMode holds the authentication type and token.
type AuthMode struct {
	Type  AuthType
	Token string
}

// Config holds SDK client configuration.
type Config struct {
	BaseURL        string
	Auth           AuthMode
	DefaultHeaders map[string]string
	TimeoutMs      int
}

// NormalizeBaseURL strips trailing slashes from a base URL.
func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/")
}

// GetAuthorizationHeader returns the Authorization header value for the auth mode.
func GetAuthorizationHeader(auth AuthMode) string {
	return "Bearer " + auth.Token
}

// ConfigFromEnv reads configuration from environment variables.
//
// Environment variables:
//   - STRAIT_BASE_URL (required)
//   - STRAIT_API_KEY (required)
//   - STRAIT_AUTH_TYPE (optional, defaults to "apiKey")
//   - STRAIT_TIMEOUT_MS (optional, defaults to 30000)
func ConfigFromEnv() (*Config, error) {
	baseURL := os.Getenv("STRAIT_BASE_URL")
	if baseURL == "" {
		return nil, &ValidationError{
			Message: "STRAIT_BASE_URL environment variable is required",
			Issues:  []string{"STRAIT_BASE_URL is not set"},
		}
	}

	apiKey := os.Getenv("STRAIT_API_KEY")
	if apiKey == "" {
		return nil, &ValidationError{
			Message: "STRAIT_API_KEY environment variable is required",
			Issues:  []string{"STRAIT_API_KEY is not set"},
		}
	}

	authType := AuthType(os.Getenv("STRAIT_AUTH_TYPE"))
	if authType == "" {
		authType = AuthTypeAPIKey
	}

	timeoutMs := 30000
	if v := os.Getenv("STRAIT_TIMEOUT_MS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, &ValidationError{
				Message: fmt.Sprintf("STRAIT_TIMEOUT_MS must be an integer, got %q", v),
				Issues:  []string{"STRAIT_TIMEOUT_MS is not a valid integer"},
			}
		}
		timeoutMs = parsed
	}

	return &Config{
		BaseURL: NormalizeBaseURL(baseURL),
		Auth: AuthMode{
			Type:  authType,
			Token: apiKey,
		},
		TimeoutMs: timeoutMs,
	}, nil
}
