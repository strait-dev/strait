package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// PolarEventIngester sends usage events to the Polar billing meter.
type PolarEventIngester struct {
	client    *http.Client
	baseURL   string
	token     string
	meterName string
	logger    *slog.Logger
}

// NewPolarEventIngester creates a new ingester. Pass the POLAR_SERVER base URL
// (e.g. "https://api.polar.sh") and POLAR_ACCESS_TOKEN.
func NewPolarEventIngester(baseURL, accessToken string, logger *slog.Logger) *PolarEventIngester {
	if logger == nil {
		logger = slog.Default()
	}
	return &PolarEventIngester{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:   baseURL,
		token:     accessToken,
		meterName: "compute_overage",
		logger:    logger,
	}
}

// polarEvent is the payload for a single Polar event ingestion.
type polarEvent struct {
	Name               string            `json:"name"`
	ExternalCustomerID string            `json:"external_customer_id"`
	Metadata           map[string]string `json:"metadata"`
	ExternalID         string            `json:"external_id"`
}

// polarIngestRequest is the top-level request body for event ingestion.
type polarIngestRequest struct {
	Events []polarEvent `json:"events"`
}

// IngestComputeUsage sends a single usage event to the Polar compute_overage meter.
// The costMicroUSD is the cost in micro-USD (1 unit = $0.000001).
// The runID is used as external_id for deduplication.
func (p *PolarEventIngester) IngestComputeUsage(ctx context.Context, polarCustomerID, runID string, costMicroUSD int64) error {
	if p.token == "" || polarCustomerID == "" {
		return nil // not configured or no customer, skip silently
	}

	events := []polarEvent{
		{
			Name:               p.meterName,
			ExternalCustomerID: polarCustomerID,
			Metadata:           map[string]string{"amount": strconv.FormatInt(costMicroUSD, 10)},
			ExternalID:         runID,
		},
	}

	return p.ingest(ctx, events)
}

// IngestBatch sends multiple usage events to Polar in a single request.
func (p *PolarEventIngester) IngestBatch(ctx context.Context, events []polarEvent) error {
	if p.token == "" || len(events) == 0 {
		return nil
	}
	return p.ingest(ctx, events)
}

func (p *PolarEventIngester) ingest(ctx context.Context, events []polarEvent) error {
	body, err := json.Marshal(polarIngestRequest{Events: events})
	if err != nil {
		return fmt.Errorf("marshaling polar events: %w", err)
	}

	url := p.baseURL + "/v1/events/ingest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating polar ingest request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending polar ingest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		p.logger.Warn("polar event ingestion failed",
			"status", resp.StatusCode,
			"event_count", len(events),
		)
		return fmt.Errorf("polar event ingestion returned status %d", resp.StatusCode)
	}

	p.logger.Debug("polar events ingested",
		"event_count", len(events),
	)
	return nil
}
