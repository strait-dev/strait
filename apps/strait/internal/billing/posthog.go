package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// PostHogClient sends server-side events to PostHog's capture API.
type PostHogClient struct {
	apiKey string
	host   string
	client *http.Client
	logger *slog.Logger
}

// NewPostHogClient creates a new PostHog client. Returns nil if apiKey is empty.
func NewPostHogClient(apiKey, host string, logger *slog.Logger) *PostHogClient {
	if apiKey == "" {
		return nil
	}
	if host == "" {
		host = "https://us.i.posthog.com"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PostHogClient{
		apiKey: apiKey,
		host:   host,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

type posthogCapturePayload struct {
	APIKey     string         `json:"api_key"`
	DistinctID string         `json:"distinct_id"`
	Event      string         `json:"event"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Capture sends a single event to PostHog. It is safe to call on a nil receiver.
func (c *PostHogClient) Capture(ctx context.Context, distinctID, event string, properties map[string]any) {
	if c == nil {
		return
	}

	payload := posthogCapturePayload{
		APIKey:     c.apiKey,
		DistinctID: distinctID,
		Event:      event,
		Properties: properties,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		c.logger.Warn("posthog: failed to marshal event", "event", event, "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/capture/", bytes.NewReader(body))
	if err != nil {
		c.logger.Warn("posthog: failed to create request", "event", event, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("posthog: failed to send event", "event", event, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.logger.Warn("posthog: capture returned error",
			"event", event,
			"status", resp.StatusCode,
		)
	}
}

// CaptureAsync sends an event in a background goroutine with its own timeout.
func (c *PostHogClient) CaptureAsync(distinctID, event string, properties map[string]any) {
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.Capture(ctx, distinctID, event, properties)
	}()
}

// CaptureRevenueEvent is a convenience method for revenue-related events.
// It uses orgID as the distinct_id (matching the PostHog group) so it links
// to the organization identified on the frontend.
func (c *PostHogClient) CaptureRevenueEvent(orgID, event string, props map[string]any) {
	if c == nil {
		return
	}
	if props == nil {
		props = map[string]any{}
	}
	// Set $groups so PostHog links this to the organization group.
	props["$groups"] = map[string]string{
		"organization": orgID,
	}
	c.CaptureAsync(fmt.Sprintf("org:%s", orgID), event, props)
}
