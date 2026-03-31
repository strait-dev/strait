package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// PolarEventIngester sends usage events to the Polar billing meter.
// Safe for concurrent use from multiple goroutines.
type PolarEventIngester struct {
	client    *http.Client
	baseURL   string
	token     string
	meterName string
	logger    *slog.Logger
	metrics   *telemetry.Metrics
}

// PolarEventIngesterOption configures a PolarEventIngester.
type PolarEventIngesterOption func(*PolarEventIngester)

// WithIngesterMetrics attaches Prometheus metrics to the ingester.
func WithIngesterMetrics(m *telemetry.Metrics) PolarEventIngesterOption {
	return func(p *PolarEventIngester) {
		p.metrics = m
	}
}

// NewPolarEventIngester creates a new ingester. Pass the POLAR_SERVER base URL
// (e.g. "https://api.polar.sh") and POLAR_ACCESS_TOKEN.
func NewPolarEventIngester(baseURL, accessToken string, logger *slog.Logger, opts ...PolarEventIngesterOption) *PolarEventIngester {
	if logger == nil {
		logger = slog.Default()
	}
	p := &PolarEventIngester{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		baseURL:   baseURL,
		token:     accessToken,
		meterName: "compute_overage",
		logger:    logger,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
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

// AgentRunBillingMeta holds metadata for an agent run billing event.
type AgentRunBillingMeta struct {
	ProjectID    string
	Model        string
	TotalTokens  int64
	CostMicrousd int64
}

// IngestAgentRunUsage sends a usage event for the agent_runs meter.
// The runID is used as external_id for deduplication.
func (p *PolarEventIngester) IngestAgentRunUsage(ctx context.Context, polarCustomerID, runID string, meta AgentRunBillingMeta) error {
	if p.token == "" || polarCustomerID == "" {
		return nil
	}

	costStr := strconv.FormatInt(meta.CostMicrousd, 10)
	if meta.CostMicrousd < 0 {
		costStr = "0"
	}

	events := []polarEvent{
		{
			Name:               "agent_runs",
			ExternalCustomerID: polarCustomerID,
			Metadata: map[string]string{
				"amount":     costStr,
				"project_id": meta.ProjectID,
				"model":      meta.Model,
				"tokens":     strconv.FormatInt(meta.TotalTokens, 10),
				"cost_micro": costStr,
			},
			ExternalID: runID,
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

	// Drain the body to allow connection reuse by the pool.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode >= 400 {
		p.logger.Warn("polar event ingestion failed",
			"status", resp.StatusCode,
			"event_count", len(events),
			"response", string(respBody),
		)
		if p.metrics != nil && p.metrics.PolarEventsDropped != nil {
			p.metrics.PolarEventsDropped.Add(ctx, int64(len(events)),
				metric.WithAttributes(attribute.String("status", "error")),
			)
		}
		return fmt.Errorf("polar event ingestion returned status %d", resp.StatusCode)
	}

	if p.metrics != nil && p.metrics.PolarEventsIngested != nil {
		p.metrics.PolarEventsIngested.Add(ctx, int64(len(events)),
			metric.WithAttributes(attribute.String("status", "ok")),
		)
	}

	p.logger.Debug("polar events ingested",
		"event_count", len(events),
	)
	return nil
}
