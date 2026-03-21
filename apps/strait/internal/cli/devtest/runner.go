// Package devtest provides local job testing without a running server.
// It simulates the Strait job execution request format by making a direct
// HTTP call to the job's endpoint URL.
package devtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// TestRequest describes a local job test to execute.
type TestRequest struct {
	JobSlug     string
	JobID       string
	EndpointURL string
	Payload     json.RawMessage
	Timeout     time.Duration
}

// TestResult holds the outcome of a local job test.
type TestResult struct {
	JobSlug    string        `json:"job_slug"`
	Endpoint   string        `json:"endpoint"`
	StatusCode int           `json:"status_code"`
	Status     string        `json:"status"`
	Duration   time.Duration `json:"duration_ms"`
	Body       string        `json:"body,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// MaxPayloadSize is the maximum allowed payload size (5MB, matching server limit).
const MaxPayloadSize = 5 * 1024 * 1024

// RunTest executes a single job test by making an HTTP POST to the endpoint.
func RunTest(ctx context.Context, req TestRequest) (*TestResult, error) {
	if req.EndpointURL == "" {
		return nil, fmt.Errorf("endpoint URL is required for job %q", req.JobSlug)
	}

	parsed, err := url.Parse(req.EndpointURL)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("endpoint URL must use http or https scheme, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("endpoint URL must have a host")
	}

	payload := req.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}

	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload exceeds maximum size of %d bytes", MaxPayloadSize)
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	runID := fmt.Sprintf("test-%d", time.Now().UnixMilli())

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Strait-Job-ID", req.JobID)
	httpReq.Header.Set("X-Strait-Job-Slug", req.JobSlug)
	httpReq.Header.Set("X-Strait-Run-ID", runID)
	httpReq.Header.Set("X-Strait-Attempt", "1")

	start := time.Now()
	resp, doErr := client.Do(httpReq)
	duration := time.Since(start)

	result := &TestResult{
		JobSlug:  req.JobSlug,
		Endpoint: req.EndpointURL,
		Duration: duration,
	}

	if doErr != nil {
		result.Error = doErr.Error()
		return result, nil
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Status = resp.Status

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*64)) // 64KB response cap
	result.Body = string(body)

	return result, nil
}
