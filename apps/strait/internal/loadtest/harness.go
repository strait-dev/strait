//go:build loadtest

package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Harness is the top-level test orchestrator. It sets up infrastructure,
// runs scenarios, and collects results.
type Harness struct {
	Config     HarnessConfig
	Pool       *pgxpool.Pool
	Redis      *redis.Client
	TestServer *TestServer
	Metrics    *MetricsCollector
	ResultsDir string
	httpClient *http.Client
}

// HarnessConfig configures the test harness.
type HarnessConfig struct {
	// StraitURL is the base URL of the Strait API under test.
	StraitURL string

	// InternalSecret is the X-Internal-Secret header value.
	InternalSecret string

	// DatabaseURL for metrics collection.
	DatabaseURL string

	// RedisURL for metrics collection.
	RedisURL string

	// TestServerPort is the port for the test HTTP server.
	TestServerPort int

	// OutputDir is where results are written.
	OutputDir string

	// MetricsInterval is how often to sample metrics.
	MetricsInterval time.Duration
}

// NewHarness creates a test harness with the given configuration.
func NewHarness(cfg HarnessConfig) *Harness {
	if cfg.TestServerPort == 0 {
		cfg.TestServerPort = 9999
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = filepath.Join("loadtest-results", time.Now().Format("2006-01-02T15-04-05"))
	}
	if cfg.MetricsInterval == 0 {
		cfg.MetricsInterval = 10 * time.Second
	}

	return &Harness{
		Config:     cfg,
		ResultsDir: cfg.OutputDir,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Setup initializes all infrastructure: DB pool, Redis, test server, metrics.
func (h *Harness) Setup(ctx context.Context) error {
	if err := os.MkdirAll(h.ResultsDir, 0o755); err != nil {
		return fmt.Errorf("creating results dir: %w", err)
	}

	// Connect to Postgres for metrics
	if h.Config.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, h.Config.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connecting to postgres: %w", err)
		}
		h.Pool = pool
	}

	// Connect to Redis for metrics
	if h.Config.RedisURL != "" {
		opts, err := redis.ParseURL(h.Config.RedisURL)
		if err != nil {
			return fmt.Errorf("parsing redis url: %w", err)
		}
		h.Redis = redis.NewClient(opts)
		if err := h.Redis.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("pinging redis: %w", err)
		}
	}

	// Start test HTTP server
	h.TestServer = NewTestServer(h.Config.TestServerPort)
	if err := h.TestServer.Start(); err != nil {
		return fmt.Errorf("starting test server: %w", err)
	}

	// Start metrics collector
	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		Pool:      h.Pool,
		Redis:     h.Redis,
		OutputDir: filepath.Join(h.ResultsDir, "raw"),
		Interval:  h.Config.MetricsInterval,
	})
	if err != nil {
		return fmt.Errorf("creating metrics collector: %w", err)
	}
	h.Metrics = mc

	if err := h.Metrics.Start(ctx); err != nil {
		return fmt.Errorf("starting metrics collector: %w", err)
	}

	return nil
}

// Teardown cleans up all infrastructure.
func (h *Harness) Teardown() error {
	var firstErr error
	setErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if h.Metrics != nil {
		setErr(h.Metrics.Stop())
	}
	if h.TestServer != nil {
		setErr(h.TestServer.Close())
	}
	if h.Pool != nil {
		h.Pool.Close()
	}
	if h.Redis != nil {
		setErr(h.Redis.Close())
	}

	return firstErr
}

// TriggerJob sends an HTTP trigger request to the Strait API.
func (h *Harness) TriggerJob(ctx context.Context, projectID, jobID string, payload any) error {
	body, err := json.Marshal(map[string]any{
		"payload": payload,
	})
	if err != nil {
		return fmt.Errorf("marshaling trigger request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/jobs/%s/trigger", h.Config.StraitURL, jobID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating trigger request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)
	req.Header.Set("X-Project-Id", projectID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("triggering job: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("trigger returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateJob creates a job via the Strait API for load testing.
func (h *Harness) CreateJob(ctx context.Context, projectID string, job JobConfig) (string, error) {
	body, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("marshaling job config: %w", err)
	}

	url := fmt.Sprintf("%s/v1/jobs", h.Config.StraitURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating job request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)
	req.Header.Set("X-Project-Id", projectID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("creating job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create job returned status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding job response: %w", err)
	}

	return result.ID, nil
}

// GetQueueStats fetches current queue statistics from the Strait API.
func (h *Harness) GetQueueStats(ctx context.Context, projectID string) (*QueueStats, error) {
	url := fmt.Sprintf("%s/v1/stats", h.Config.StraitURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating stats request: %w", err)
	}

	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)
	req.Header.Set("X-Project-Id", projectID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching stats: %w", err)
	}
	defer resp.Body.Close()

	var stats QueueStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decoding stats: %w", err)
	}

	return &stats, nil
}

// WriteResult writes a result object to a JSON file.
func (h *Harness) WriteResult(filename string, result any) error {
	path := filepath.Join(h.ResultsDir, filename)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// JobConfig defines a job for load testing.
type JobConfig struct {
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	EndpointURL   string `json:"endpoint_url,omitempty"`
	ExecutionMode string `json:"execution_mode"`
	ImageURI      string `json:"image_uri,omitempty"`
	MachinePreset string `json:"machine_preset,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`
	TimeoutSecs   int    `json:"timeout_secs,omitempty"`
}

// QueueStats represents queue statistics from the API.
type QueueStats struct {
	Queued    int64 `json:"queued"`
	Executing int64 `json:"executing"`
	Delayed   int64 `json:"delayed"`
	Failed    int64 `json:"failed"`
	Completed int64 `json:"completed"`
}

// QueueDepth returns the total number of items waiting in the queue.
func (qs *QueueStats) QueueDepth() int64 {
	return qs.Queued + qs.Delayed
}
