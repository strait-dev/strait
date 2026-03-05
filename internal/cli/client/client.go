package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"orchestrator/internal/domain"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type CreateJobRequest struct {
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description,omitempty"`
	Cron        string          `json:"cron,omitempty"`
	EndpointURL string          `json:"endpoint_url"`
	MaxAttempts int             `json:"max_attempts,omitempty"`
	TimeoutSecs int             `json:"timeout_secs,omitempty"`
	RunTTLSecs  int             `json:"run_ttl_secs,omitempty"`
	Schema      json.RawMessage `json:"payload_schema,omitempty"`
}

type TriggerJobRequest struct {
	Payload     json.RawMessage `json:"payload,omitempty"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	Priority    int             `json:"priority,omitempty"`
}

type TriggerJobResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	RunToken string `json:"run_token,omitempty"`
}

type HealthStatus struct {
	Status string `json:"status"`
}

func New(baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base URL must be http(s)")
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: parsed.String(),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []domain.Job
	if err := c.doJSON(ctx, http.MethodGet, "/v1/jobs", query, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	var out domain.Job
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/jobs", id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateJob(ctx context.Context, req CreateJobRequest) (*domain.Job, error) {
	var out domain.Job
	if err := c.doJSON(ctx, http.MethodPost, "/v1/jobs", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) TriggerJob(ctx context.Context, jobID string, req TriggerJobRequest, idempotencyKey string) (*TriggerJobResponse, error) {
	var out TriggerJobResponse
	headers := map[string]string{}
	if strings.TrimSpace(idempotencyKey) != "" {
		headers["X-Idempotency-Key"] = strings.TrimSpace(idempotencyKey)
	}
	if err := c.doJSONWithHeaders(ctx, http.MethodPost, path.Join("/v1/jobs", jobID, "trigger"), nil, req, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListRuns(ctx context.Context, projectID, status string, limit int) ([]domain.JobRun, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if strings.TrimSpace(status) != "" {
		query.Set("status", strings.TrimSpace(status))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}

	var out []domain.JobRun
	if err := c.doJSON(ctx, http.MethodGet, "/v1/runs", query, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	var out domain.JobRun
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/runs", runID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	var out domain.JobRun
	if err := c.doJSON(ctx, http.MethodDelete, path.Join("/v1/runs", runID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListRunEvents(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
	query := url.Values{}
	if strings.TrimSpace(level) != "" {
		query.Set("level", strings.TrimSpace(level))
	}
	if strings.TrimSpace(eventType) != "" {
		query.Set("type", strings.TrimSpace(eventType))
	}

	var out []domain.RunEvent
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/runs", runID, "events"), query, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	var out HealthStatus
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) HealthReady(ctx context.Context) (*HealthStatus, error) {
	var out HealthStatus
	if err := c.doJSON(ctx, http.MethodGet, "/health/ready", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, body any, out any) error {
	return c.doJSONWithHeaders(ctx, method, endpoint, query, body, nil, out)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, endpoint string, query url.Values, body any, headers map[string]string, out any) error {
	fullURL, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	fullURL.Path = path.Join(fullURL.Path, endpoint)
	if query != nil {
		fullURL.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req) //nolint:gosec // endpoint is configured by explicit CLI input and validated in constructor
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if msg, ok := apiErr["error"].(string); ok && msg != "" {
			return fmt.Errorf("request failed (%d): %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
