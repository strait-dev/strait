package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"fmt"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/danielgtaylor/huma/v2"
)

type testWebhookRequest struct {
	URL    string `json:"url" validate:"required,url"`
	Secret string `json:"secret,omitempty"`
}

type TestWebhookInput struct {
	Body testWebhookRequest
}

type TestWebhookOutput struct {
	Body map[string]any
}

func (s *Server) handleTestWebhook(ctx context.Context, input *TestWebhookInput) (*TestWebhookOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := validateURLWithTLS(req.URL, s.config.WebhookRequireTLS); err != nil {
		slog.Warn("webhook test URL rejected", "url", httputil.RedactURLForLog(req.URL), "error", err)
		return nil, huma.Error400BadRequest("invalid webhook URL")
	}
	testPayload, _ := json.Marshal(map[string]any{
		"type":      "webhook.test",
		"timestamp": time.Now().UTC(),
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, bytes.NewReader(testPayload))
	if err != nil {
		return nil, huma.Error400BadRequest("failed to create request: " + err.Error())
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Strait-Webhook-Test/1.0")
	if req.Secret != "" {
		ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		payload := append([]byte(ts+"."), testPayload...)
		mac := hmac.New(sha256.New, []byte(req.Secret))
		_, _ = mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		httpReq.Header.Set("X-Strait-Timestamp", ts)
		httpReq.Header.Set("X-Strait-Signature", "v1="+sig)
	}
	start := time.Now()
	// Re-validate the URL on every hop. Without CheckRedirect the bare http
	// client follows 3xx by default — a public attacker host can return 302
	// to http://169.254.169.254/ (cloud metadata) and exfiltrate IAM creds.
	// The SSRF guard at the entry point only covers the first hop; this
	// hook covers every redirect target.
	requireTLS := s.config != nil && s.config.WebhookRequireTLS
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: httputil.NewExternalTransport(s.config != nil && s.config.AllowPrivateEndpoints),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			if err := validateURLWithTLS(req.URL.String(), requireTLS); err != nil {
				return fmt.Errorf("redirect blocked by ssrf guard: %w", err)
			}
			return nil
		},
	}
	resp, err := client.Do(httpReq)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		// Audit failed attempts too. SSRF-guard blocks and connection errors land
		// here; without this, an operator probing internal hosts (all rejected by
		// the guard) would leave no trace in the audit trail.
		s.emitAuditEvent(ctx, domain.AuditActionWebhookTested, "webhook", "", map[string]any{
			"url_host": urlHost(req.URL),
			"success":  false,
			"error":    "connection_failed",
		})
		return &TestWebhookOutput{Body: map[string]any{
			"success":    false,
			"error":      "connection to webhook URL failed",
			"latency_ms": latencyMs,
		}}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	s.emitAuditEvent(ctx, domain.AuditActionWebhookTested, "webhook", "", map[string]any{
		"url_host":    urlHost(req.URL),
		"status_code": resp.StatusCode,
		"success":     resp.StatusCode >= 200 && resp.StatusCode < 300,
	})

	return &TestWebhookOutput{Body: map[string]any{
		"success":       resp.StatusCode >= 200 && resp.StatusCode < 300,
		"status_code":   resp.StatusCode,
		"latency_ms":    latencyMs,
		"response_body": string(body),
	}}, nil
}

// urlHost parses a URL and returns only its host component. Returns empty
// string on parse failure. Used for audit events to avoid leaking query
// strings or path segments that may contain secrets.
func urlHost(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

type ReplayWebhookDeliveryInput struct {
	ID string `path:"id"`
}

type ReplayWebhookDeliveryOutput struct {
	Body any
}

func (s *Server) handleReplayWebhookDelivery(ctx context.Context, input *ReplayWebhookDeliveryInput) (*ReplayWebhookDeliveryOutput, error) {
	original, err := s.store.GetWebhookDelivery(ctx, projectIDFromContext(ctx), input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("webhook delivery not found")
	}
	if original.JobID != "" {
		job, jobErr := s.store.GetJob(ctx, original.JobID)
		if jobErr != nil || job == nil || job.ProjectID != projectIDFromContext(ctx) {
			return nil, huma.Error404NotFound("webhook delivery not found")
		}
		if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
			return nil, huma.Error404NotFound("webhook delivery not found")
		}
	} else {
		if original.ProjectID == "" || original.ProjectID != projectIDFromContext(ctx) {
			return nil, huma.Error404NotFound("webhook delivery not found")
		}
		if environmentIDFromContext(ctx) != "" {
			return nil, huma.Error404NotFound("webhook delivery not found")
		}
	}
	replay, err := s.store.ReplayWebhookDelivery(ctx, projectIDFromContext(ctx), input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create replay delivery")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWebhookDeliveryReplayed, "webhook", input.ID, map[string]any{
		"original_delivery_id": input.ID,
		"job_id":               original.JobID,
	})

	return &ReplayWebhookDeliveryOutput{Body: sanitizeWebhookDeliveryResponsePtr(replay)}, nil
}
