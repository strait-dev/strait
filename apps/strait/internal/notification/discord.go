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

// DiscordSender sends notifications via Discord webhooks.
type DiscordSender struct {
	client *http.Client
}

// NewDiscordSender creates a new DiscordSender with the given HTTP client.
func NewDiscordSender(client *http.Client) *DiscordSender {
	if client == nil {
		client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: httputil.NewExternalTransport(false),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &DiscordSender{client: client}
}

type discordConfig struct {
	WebhookURL string `json:"webhook_url"`
}

func (d *DiscordSender) Send(ctx context.Context, channel *domain.NotificationChannel, delivery *domain.NotificationDelivery) error {
	var cfg discordConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("parse discord config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("discord webhook_url is empty")
	}
	if err := httputil.ValidateExternalURL(cfg.WebhookURL); err != nil {
		return fmt.Errorf("discord webhook_url rejected: %w", err)
	}

	body, err := marshalDiscordPayload(delivery.EventType, delivery.Payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send discord notification: %s", sanitizeDeliveryError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
