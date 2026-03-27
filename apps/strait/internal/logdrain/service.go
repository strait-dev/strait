package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"strait/internal/domain"
)

// ProtectedHeaders are HTTP headers that must not be overridden by
// user-provided auth config to prevent request smuggling and
// header injection attacks.
var ProtectedHeaders = map[string]bool{
	"host":              true,
	"content-length":    true,
	"content-type":      true,
	"transfer-encoding": true,
	"connection":        true,
	"upgrade":           true,
	"te":                true,
	"trailer":           true,
}

// jsonMarshal is the JSON marshaling function, replaceable in tests.
var jsonMarshal = json.Marshal

// Service dispatches run events to log drain endpoints.
type Service struct {
	client *http.Client
}

func NewService() *Service {
	return &Service{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// DrainRunEvents sends a batch of events to the drain endpoint.
func (s *Service) DrainRunEvents(ctx context.Context, drain *domain.LogDrain, events []domain.RunEvent) error {
	payload, err := jsonMarshal(events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, drain.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	switch drain.AuthType {
	case "bearer":
		if token, ok := drain.AuthConfig["token"]; ok {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case "basic":
		user := drain.AuthConfig["username"]
		pass := drain.AuthConfig["password"]
		req.SetBasicAuth(user, pass)
	case "header":
		for k, v := range drain.AuthConfig {
			if ProtectedHeaders[strings.ToLower(k)] {
				continue
			}
			req.Header.Set(k, v)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("drain request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("drain endpoint returned %d", resp.StatusCode)
	}
	return nil
}
