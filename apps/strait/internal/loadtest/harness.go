//go:build loadtest

package loadtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// ExecutionMode controls which type of jobs the harness creates and triggers.
type ExecutionMode string

const (
	// ModeHTTP creates only HTTP endpoint jobs. The harness configures each
	// job with an endpoint signing secret, so Strait signs dispatch payloads
	// and the target server verifies them before accepting the request.
	ModeHTTP ExecutionMode = "http"

	// ModeWorker creates worker-mode jobs and drives load via the gRPC
	// WorkerService streaming RPC rather than outbound HTTP dispatch.
	// Workers connect to the server, claim runs from the queue, and report
	// results back over the same bidirectional stream.
	// See internal/loadtest/worker_scenario.go for the implementation.
	ModeWorker ExecutionMode = "worker"
)

const maxStatsResponseBytes = 1 << 20

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
	jobIDsMu   sync.RWMutex
	jobIDs     map[string]string
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

	// ExecutionMode controls which job types are created.
	ExecutionMode ExecutionMode

	// WorkerConfig holds gRPC worker scenario parameters for ModeWorker runs.
	// When nil, DefaultWorkerConfig() is used.
	WorkerConfig *WorkerConfig

	// EndpointSigningSecret signs HTTP dispatches sent to the load-test
	// receiver. When empty, NewHarness generates a per-run secret.
	EndpointSigningSecret string
}

// NewHarness creates a test harness with the given configuration.
func NewHarness(cfg HarnessConfig) *Harness {
	if cfg.TestServerPort == 0 {
		cfg.TestServerPort = 9000
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = filepath.Join("loadtest-results", time.Now().Format("2006-01-02T15-04-05"))
	}
	if cfg.MetricsInterval == 0 {
		cfg.MetricsInterval = 10 * time.Second
	}
	if cfg.EndpointSigningSecret == "" {
		cfg.EndpointSigningSecret = generateLoadTestSecret()
	}

	return &Harness{
		Config:     cfg,
		ResultsDir: cfg.OutputDir,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          500,
				MaxIdleConnsPerHost:   500,
				MaxConnsPerHost:       500,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
	}
}

var loadtestRandRead = rand.Read

func generateLoadTestSecret() string {
	var b [32]byte
	if _, err := loadtestRandRead(b[:]); err != nil {
		panic("generate loadtest endpoint signing secret: " + err.Error())
	}
	return "loadtest_" + hex.EncodeToString(b[:])
}

// Setup initializes all infrastructure: DB pool, Redis, test server, metrics.
func (h *Harness) Setup(ctx context.Context) error {
	if err := os.MkdirAll(h.ResultsDir, 0o750); err != nil {
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
	h.TestServer = NewTestServer(h.Config.TestServerPort, WithTestServerHMACSecret(h.Config.EndpointSigningSecret))
	if err := h.TestServer.Start(); err != nil {
		return fmt.Errorf("starting test server: %w", err)
	}

	// Start metrics collector
	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		Pool:      h.Pool,
		Redis:     h.Redis,
		OutputDir: filepath.Join(h.ResultsDir, "raw"),
		Interval:  h.Config.MetricsInterval,
		Harness:   h,
		ProjectID: "loadtest-project",
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

// Teardown cleans up all infrastructure and flushes queued runs.
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

	// Flush queued/running runs left over from load tests
	if h.Pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := h.Pool.Exec(ctx, "DELETE FROM job_runs WHERE status IN ('queued', 'running') AND project_id = 'loadtest-project'")
		cancel()
		setErr(err)
	}

	if h.Pool != nil {
		h.Pool.Close()
	}
	if h.Redis != nil {
		setErr(h.Redis.Close())
	}

	// Close idle HTTP connections to allow clean shutdown
	h.httpClient.CloseIdleConnections()

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
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("trigger returned status %d", resp.StatusCode)
	}

	return nil
}

// ResolveJobID returns the setup-created job ID for a standard load-test slug.
// It falls back to the input so ad hoc tests that create jobs independently keep
// working, but standard scenarios should call SetupLoadTestJobs first.
func (h *Harness) ResolveJobID(slug string) string {
	if h == nil {
		return slug
	}
	h.jobIDsMu.RLock()
	defer h.jobIDsMu.RUnlock()
	if h.jobIDs == nil {
		return slug
	}
	if id, ok := h.jobIDs[slug]; ok {
		return id
	}
	return slug
}

// CreateJob creates a job via the Strait API for load testing.
func (h *Harness) CreateJob(ctx context.Context, projectID string, job JobConfig) (string, error) {
	if err := validateLoadTestEndpointURL(job.EndpointURL); err != nil {
		return "", err
	}

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

func validateLoadTestEndpointURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid load test endpoint URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid load test endpoint URL scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("invalid load test endpoint URL: missing host")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return fmt.Errorf("load test endpoint URL must not advertise wildcard host %q", host)
	}
	return nil
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxStatsResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading stats response: %w", err)
	}
	if len(body) > maxStatsResponseBytes {
		return nil, fmt.Errorf("stats response exceeded %d bytes", maxStatsResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("stats returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var errorEnvelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &errorEnvelope); err == nil && errorEnvelope.Error != "" {
		return nil, fmt.Errorf("stats returned error payload: %s", errorEnvelope.Error)
	}

	var stats QueueStats
	if err := json.Unmarshal(body, &stats); err != nil {
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
	return os.WriteFile(path, data, 0o600)
}

// JobConfig defines a job for load testing.
type JobConfig struct {
	ProjectID             string `json:"project_id"`
	Name                  string `json:"name"`
	Slug                  string `json:"slug"`
	EndpointURL           string `json:"endpoint_url,omitempty"`
	EndpointSigningSecret string `json:"endpoint_signing_secret,omitempty"`
	ExecutionMode         string `json:"execution_mode"`
	MaxAttempts           int    `json:"max_attempts,omitempty"`
	TimeoutSecs           int    `json:"timeout_secs,omitempty"`
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

// SetupLoadTestJobs creates the standard HTTP load test jobs and returns their IDs.
// The test server must be started before calling this.
func (h *Harness) SetupLoadTestJobs(ctx context.Context, projectID string) (map[string]string, error) {
	testServerURL := fmt.Sprintf("http://%s", h.TestServer.Addr())
	jobs := map[string]string{}
	signingSecret := h.Config.EndpointSigningSecret

	httpConfigs := standardLoadTestJobConfigs(projectID, testServerURL, signingSecret)
	if err := h.createJobs(ctx, projectID, httpConfigs, jobs); err != nil {
		return nil, err
	}

	h.jobIDsMu.Lock()
	h.jobIDs = make(map[string]string, len(jobs))
	for slug, id := range jobs {
		h.jobIDs[slug] = id
	}
	h.jobIDsMu.Unlock()

	return jobs, nil
}

func standardLoadTestJobConfigs(projectID, testServerURL, signingSecret string) []JobConfig {
	return []JobConfig{
		{
			ProjectID:             projectID,
			Name:                  "Load Test Fast Echo",
			Slug:                  "loadtest-fast-echo",
			EndpointURL:           testServerURL + "/fast-echo",
			EndpointSigningSecret: signingSecret,
			ExecutionMode:         "http",
			MaxAttempts:           1,
			TimeoutSecs:           30,
		},
		{
			ProjectID:             projectID,
			Name:                  "Load Test Slow Process",
			Slug:                  "loadtest-slow-process",
			EndpointURL:           testServerURL + "/slow-process",
			EndpointSigningSecret: signingSecret,
			ExecutionMode:         "http",
			MaxAttempts:           1,
			TimeoutSecs:           60,
		},
		{
			ProjectID:             projectID,
			Name:                  "Load Test Variable Load",
			Slug:                  "loadtest-variable-load",
			EndpointURL:           testServerURL + "/variable-load",
			EndpointSigningSecret: signingSecret,
			ExecutionMode:         "http",
			MaxAttempts:           1,
			TimeoutSecs:           30,
		},
		{
			ProjectID:             projectID,
			Name:                  "Load Test Flaky",
			Slug:                  "loadtest-flaky",
			EndpointURL:           testServerURL + "/flaky",
			EndpointSigningSecret: signingSecret,
			ExecutionMode:         "http",
			MaxAttempts:           3,
			TimeoutSecs:           30,
		},
		{
			ProjectID:             projectID,
			Name:                  "Load Test Error Scenarios",
			Slug:                  "loadtest-errors",
			EndpointURL:           testServerURL + "/error-scenario",
			EndpointSigningSecret: signingSecret,
			ExecutionMode:         "http",
			MaxAttempts:           1,
			TimeoutSecs:           30,
		},
	}
}

// createJobs creates the given job configs, falling back to slug lookup on conflict.
func (h *Harness) createJobs(ctx context.Context, projectID string, configs []JobConfig, dest map[string]string) error {
	for _, cfg := range configs {
		id, err := h.CreateJob(ctx, projectID, cfg)
		if err != nil {
			existingID, findErr := h.FindJobBySlug(ctx, projectID, cfg.Slug)
			if findErr != nil {
				return fmt.Errorf("creating job %s: %w (and failed to find existing: %w)", cfg.Slug, err, findErr)
			}
			dest[cfg.Slug] = existingID
			continue
		}
		dest[cfg.Slug] = id
	}
	return nil
}

// terminalStatuses contains all run statuses that indicate a run has finished.
var terminalStatuses = map[string]bool{
	"completed":     true,
	"failed":        true,
	"dead_letter":   true,
	"timed_out":     true,
	"crashed":       true,
	"system_failed": true,
	"canceled":      true,
}

// TriggerAndWait triggers a job and polls until the run reaches a terminal state.
// Returns the run ID, final status, and time from trigger to completion.
func (h *Harness) TriggerAndWait(ctx context.Context, projectID, jobID string, payload any, timeout time.Duration) (runID string, status string, elapsed time.Duration, err error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Trigger the job and parse the run ID from the response.
	body, err := json.Marshal(map[string]any{
		"payload": payload,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("marshaling trigger request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/jobs/%s/trigger", h.Config.StraitURL, jobID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", "", 0, fmt.Errorf("creating trigger request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)
	req.Header.Set("X-Project-Id", projectID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("triggering job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", 0, fmt.Errorf("trigger returned status %d: %s", resp.StatusCode, respBody)
	}

	var triggerResp struct {
		RunID string `json:"run_id"`
		ID    string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&triggerResp); err != nil {
		return "", "", 0, fmt.Errorf("decoding trigger response: %w", err)
	}

	runID = triggerResp.RunID
	if runID == "" {
		runID = triggerResp.ID
	}
	if runID == "" {
		return "", "", 0, fmt.Errorf("trigger response missing run ID")
	}

	// Poll until terminal state.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return runID, "", time.Since(start), fmt.Errorf("timed out waiting for run %s: %w", runID, ctx.Err())
		case <-ticker.C:
			status, err = h.GetRun(ctx, runID)
			if err != nil {
				return runID, "", time.Since(start), fmt.Errorf("polling run %s: %w", runID, err)
			}
			if terminalStatuses[status] {
				return runID, status, time.Since(start), nil
			}
		}
	}
}

// GetRun fetches the current status of a single run.
func (h *Harness) GetRun(ctx context.Context, runID string) (status string, err error) {
	url := fmt.Sprintf("%s/v1/runs/%s", h.Config.StraitURL, runID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating run request: %w", err)
	}

	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get run returned status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding run response: %w", err)
	}

	return result.Status, nil
}

// FindJobBySlug finds a job by slug and returns its UUID.
func (h *Harness) FindJobBySlug(ctx context.Context, projectID, slug string) (string, error) {
	url := fmt.Sprintf("%s/v1/jobs?slug=%s", h.Config.StraitURL, url.QueryEscape(slug))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Internal-Secret", h.Config.InternalSecret)
	req.Header.Set("X-Project-Id", projectID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, j := range result.Data {
		if j.Slug == slug {
			return j.ID, nil
		}
	}

	return "", fmt.Errorf("job with slug %q not found", slug)
}
