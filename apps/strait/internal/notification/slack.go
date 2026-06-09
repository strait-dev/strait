package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
)

// SlackSender sends notifications via Slack incoming webhooks.
type SlackSender struct {
	client *http.Client
}

// NewSlackSender creates a new SlackSender with the given HTTP client.
func NewSlackSender(client *http.Client) *SlackSender {
	if client == nil {
		client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: httputil.NewExternalTransport(false),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &SlackSender{client: client}
}

type slackConfig struct {
	WebhookURL string `json:"webhook_url"`
}

func (s *SlackSender) Send(ctx context.Context, channel *domain.NotificationChannel, delivery *domain.NotificationDelivery) error {
	var cfg slackConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("parse slack config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("slack webhook_url is empty")
	}
	if err := httputil.ValidateExternalURL(cfg.WebhookURL); err != nil {
		return fmt.Errorf("slack webhook_url rejected: %w", err)
	}

	body, err := marshalSlackPayload(delivery.EventType, delivery.Payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send slack notification: %s", sanitizeDeliveryError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}
