package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"
)

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
	payload, err := json.Marshal(events)
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
