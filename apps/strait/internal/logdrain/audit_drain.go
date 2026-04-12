package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
)

// AuditSIEMDrain forwards audit events to an external SIEM endpoint
// as NDJSON over HTTP POST. Each batch is sent with a Bearer token.
type AuditSIEMDrain struct {
	endpoint  string
	authToken string
	client    *http.Client
	logger    *slog.Logger
}

// NewAuditSIEMDrain creates a new SIEM drain. Returns nil if endpoint is empty.
func NewAuditSIEMDrain(endpoint, authToken string) *AuditSIEMDrain {
	if endpoint == "" {
		return nil
	}
	return &AuditSIEMDrain{
		endpoint:  endpoint,
		authToken: authToken,
		client:    &http.Client{Timeout: 30 * time.Second},
		logger:    slog.Default(),
	}
}

// ForwardBatch sends a slice of audit events to the SIEM endpoint as NDJSON.
func (d *AuditSIEMDrain) ForwardBatch(ctx context.Context, events []domain.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("encode audit event %d: %w", i, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create SIEM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("User-Agent", "Strait-Audit-SIEM/1.0")
	if d.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.authToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("SIEM request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM returned status %d", resp.StatusCode)
	}

	d.logger.Info("audit events forwarded to SIEM",
		"count", len(events), "endpoint", d.endpoint, "status", resp.StatusCode)
	return nil
}
