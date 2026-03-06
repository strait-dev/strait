package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const serviceName = "orchestrator"

func KeyName(contextName string) string {
	name := strings.TrimSpace(contextName)
	if name == "" {
		return "default"
	}
	return name
}

func SaveAPIKey(contextName, apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("api key cannot be empty")
	}
	return keyring.Set(serviceName, KeyName(contextName), apiKey)
}

func LoadAPIKey(contextName string) (string, error) {
	return keyring.Get(serviceName, KeyName(contextName))
}

func DeleteAPIKey(contextName string) error {
	err := keyring.Delete(serviceName, KeyName(contextName))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func ValidateAPIKey(ctx context.Context, serverURL, apiKey string, timeout time.Duration) error {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if base == "" {
		return fmt.Errorf("server URL is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("server URL must be a valid http(s) URL")
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("api key is required")
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String()+"/v1/stats", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api key validation failed with status %d", resp.StatusCode)
	}

	return nil
}
