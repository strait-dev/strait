//go:build loadtest

package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Globals

var (
	testEnv   *testutil.TestEnv
	testStore *store.Queries
	testQueue *queue.PgQueQueue
	ts        *httptest.Server
	baseURL   string
	cfg       loadCfg
)

// httpClient is used for seeding data via HTTP.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Configuration (overridable via env vars)

type loadCfg struct {
	// Baseline mode: fixed-rate SLA validation.
	BaselineRate     int
	BaselineDuration time.Duration

	// Stress mode: high-rate throughput ceiling discovery.
	StressRate     int
	StressWorkers  uint64
	StressDuration time.Duration

	// Spike mode: ramp-up to discover degradation point.
	SpikeStartRate int
	SpikeSlope     int
	SpikeDuration  time.Duration

	// SLA thresholds (generous defaults for CI).
	MaxP95         time.Duration
	MaxP99         time.Duration
	MinSuccessRate float64
}

func loadCfgFromEnv() loadCfg {
	return loadCfg{
		BaselineRate:     envInt("LOADTEST_BASELINE_RATE", 100),
		BaselineDuration: envDuration("LOADTEST_BASELINE_DURATION", 10*time.Second),
		StressRate:       envInt("LOADTEST_STRESS_RATE", 5000),
		StressWorkers:    uint64(envInt("LOADTEST_STRESS_WORKERS", 50)),
		StressDuration:   envDuration("LOADTEST_STRESS_DURATION", 15*time.Second),
		SpikeStartRate:   envInt("LOADTEST_SPIKE_START_RATE", 10),
		SpikeSlope:       envInt("LOADTEST_SPIKE_SLOPE", 30),
		SpikeDuration:    envDuration("LOADTEST_SPIKE_DURATION", 20*time.Second),
		MaxP95:           envDuration("LOADTEST_MAX_P95", 500*time.Millisecond),
		MaxP99:           envDuration("LOADTEST_MAX_P99", 1*time.Second),
		MinSuccessRate:   envFloat("LOADTEST_MIN_SUCCESS_RATE", 0.95),
	}
}

// TestMain

func TestMain(m *testing.M) {
	cfg = loadCfgFromEnv()
	ctx := context.Background()

	var err error
	testEnv, err = testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("loadtest: setup test env: %v", err)
	}

	testStore = store.New(testEnv.DB.Pool)
	testStore.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	testQueue = queue.NewPgQueQueue(testEnv.DB.Pool, queue.NewPostgresRunWriter(testEnv.DB.Pool), queue.PgQueConfig{})
	tickerCtx, cancelTicker := context.WithCancel(ctx)
	go testQueue.RunTicker(tickerCtx)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:           "test-secret-value",
			JWTSigningKey:            testJWTSigningKey,
			SecretEncryptionKey:      "test-encryption-key-32bytes!!!!",
			RateLimitRequests:        0, // disabled for load testing
			RateLimitWindow:          time.Minute,
			TriggerRateLimitRequests: 0,
			TriggerRateLimitWindow:   time.Minute,
			CORSAllowedOrigins:       []string{"*"},
			CORSAllowCredentials:     false,
		},
		Store: testStore,
		Queue: testQueue,
	})

	ts = httptest.NewServer(srv)
	baseURL = ts.URL

	code := m.Run()

	cancelTicker()
	ts.Close()
	testEnv.Cleanup(ctx)
	os.Exit(code)
}

// Attack mode helpers

type attackOption func(*attackSettings)

type attackSettings struct {
	rate     int
	workers  uint64
	duration time.Duration
}

// WithRate overrides the default request rate.
func withRate(r int) attackOption { return func(s *attackSettings) { s.rate = r } }

// WithWorkers overrides the default worker count.
func withWorkers(w uint64) attackOption { return func(s *attackSettings) { s.workers = w } }

// WithDuration overrides the default attack duration.
func withDuration(d time.Duration) attackOption { return func(s *attackSettings) { s.duration = d } }

// runBaseline runs a fixed-rate attack for SLA validation.
// Asserts latencies stay within configured thresholds at a sustained rate.
func runBaseline(t *testing.T, name string, tgt vegeta.Targeter, opts ...attackOption) vegeta.Metrics {
	t.Helper()
	s := attackSettings{rate: cfg.BaselineRate, duration: cfg.BaselineDuration, workers: 10}
	for _, o := range opts {
		o(&s)
	}
	pacer := vegeta.ConstantPacer{Freq: s.rate, Per: time.Second}
	return doAttack(t, name+"/baseline", tgt, pacer, s.duration, s.workers)
}

// runStress runs a high-rate attack to discover throughput ceiling.
// Workers become the bottleneck — measures max achievable throughput.
func runStress(t *testing.T, name string, tgt vegeta.Targeter, opts ...attackOption) vegeta.Metrics {
	t.Helper()
	s := attackSettings{rate: cfg.StressRate, workers: cfg.StressWorkers, duration: cfg.StressDuration}
	for _, o := range opts {
		o(&s)
	}
	pacer := vegeta.ConstantPacer{Freq: s.rate, Per: time.Second}
	return doAttack(t, name+"/stress", tgt, pacer, s.duration, s.workers)
}

// runSpike runs a linearly ramping attack to find the degradation point.
func runSpike(t *testing.T, name string, tgt vegeta.Targeter, opts ...attackOption) vegeta.Metrics {
	t.Helper()
	s := attackSettings{workers: cfg.StressWorkers, duration: cfg.SpikeDuration}
	for _, o := range opts {
		o(&s)
	}
	pacer := vegeta.LinearPacer{
		StartAt: vegeta.Rate{Freq: cfg.SpikeStartRate, Per: time.Second},
		Slope:   float64(cfg.SpikeSlope),
	}
	return doAttack(t, name+"/spike", tgt, pacer, s.duration, s.workers)
}

// doAttack executes a Vegeta attack and collects metrics.
func doAttack(t *testing.T, name string, tgt vegeta.Targeter, pacer vegeta.Pacer, dur time.Duration, workers uint64) vegeta.Metrics {
	t.Helper()
	atk := vegeta.NewAttacker(
		vegeta.Workers(workers),
		vegeta.MaxWorkers(workers),
		vegeta.Timeout(30*time.Second),
	)

	var m vegeta.Metrics
	for res := range atk.Attack(tgt, pacer, dur, name) {
		m.Add(res)
	}
	m.Close()
	logMetrics(t, name, m)
	return m
}

// Assertions

// assertLatencySLA checks p95 and p99 against configured SLA thresholds.
func assertLatencySLA(t *testing.T, m vegeta.Metrics) {
	t.Helper()
	if m.Latencies.P95 > cfg.MaxP95 {
		t.Errorf("p95 latency %v exceeds SLA %v", m.Latencies.P95, cfg.MaxP95)
	}
	if m.Latencies.P99 > cfg.MaxP99 {
		t.Errorf("p99 latency %v exceeds SLA %v", m.Latencies.P99, cfg.MaxP99)
	}
}

// assertSuccessRate checks that the success ratio meets the minimum.
func assertSuccessRate(t *testing.T, m vegeta.Metrics, minRate float64) {
	t.Helper()
	if m.Success < minRate {
		t.Errorf("success rate %.4f below minimum %.4f", m.Success, minRate)
	}
}

// assertStatusCodes checks that only the specified status codes appear.
func assertStatusCodes(t *testing.T, m vegeta.Metrics, allowed ...string) {
	t.Helper()
	allowedSet := make(map[string]bool, len(allowed))
	for _, c := range allowed {
		allowedSet[c] = true
	}
	for code, count := range m.StatusCodes {
		if !allowedSet[code] {
			t.Errorf("unexpected status code %s (%d occurrences)", code, count)
		}
	}
}

// assertNoServerErrors checks that no 5xx responses were received.
func assertNoServerErrors(t *testing.T, m vegeta.Metrics) {
	t.Helper()
	for code, count := range m.StatusCodes {
		if len(code) > 0 && code[0] == '5' {
			t.Errorf("server error %s occurred %d times", code, count)
		}
	}
}

// logMetrics prints key metrics for debugging and CI visibility.
func logMetrics(t *testing.T, name string, m vegeta.Metrics) {
	t.Helper()
	t.Logf("--- %s ---", name)
	t.Logf("  requests:   %d", m.Requests)
	t.Logf("  rate:       %.2f rps", m.Rate)
	t.Logf("  throughput: %.2f rps (successful)", m.Throughput)
	t.Logf("  success:    %.4f (%.2f%%)", m.Success, m.Success*100)
	t.Logf("  latencies:  p50=%v p95=%v p99=%v max=%v",
		m.Latencies.P50, m.Latencies.P95, m.Latencies.P99, m.Latencies.Max)

	if len(m.StatusCodes) > 0 {
		parts := make([]string, 0, len(m.StatusCodes))
		for code, count := range m.StatusCodes {
			parts = append(parts, fmt.Sprintf("%s:%d", code, count))
		}
		t.Logf("  codes:      %s", strings.Join(parts, " "))
	}

	if len(m.Errors) > 0 {
		maxShow := min(len(m.Errors), 5)
		for _, e := range m.Errors[:maxShow] {
			t.Logf("  error:      %s", e)
		}
		if len(m.Errors) > 5 {
			t.Logf("  ... and %d more errors", len(m.Errors)-5)
		}
	}
}

// Targeter factories

// newTargeter creates a Vegeta targeter for authenticated API requests.
func newTargeter(method, path string, bodyFn func() []byte) vegeta.Targeter {
	return func(tgt *vegeta.Target) error {
		tgt.Method = method
		tgt.URL = baseURL + path
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		if bodyFn != nil {
			tgt.Body = bodyFn()
		}
		return nil
	}
}

// newProjectTargeter creates a targeter that sets X-Project-Id header for project-scoped endpoints.
func newProjectTargeter(method, path, projectID string, bodyFn func() []byte) vegeta.Targeter {
	return func(tgt *vegeta.Target) error {
		tgt.Method = method
		tgt.URL = baseURL + path
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"X-Project-Id":      []string{projectID},
			"Content-Type":      []string{"application/json"},
		}
		if bodyFn != nil {
			tgt.Body = bodyFn()
		}
		return nil
	}
}

// newSDKTargeter creates a targeter with Bearer token auth for SDK endpoints.
func newSDKTargeter(method, path, token string, bodyFn func() []byte) vegeta.Targeter {
	return func(tgt *vegeta.Target) error {
		tgt.Method = method
		tgt.URL = baseURL + path
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + token},
			"Content-Type":  []string{"application/json"},
		}
		if bodyFn != nil {
			tgt.Body = bodyFn()
		}
		return nil
	}
}

// newAPIKeyTargeter creates a targeter authenticated with an API key.
func newAPIKeyTargeter(method, path, apiKey string, bodyFn func() []byte) vegeta.Targeter {
	return func(tgt *vegeta.Target) error {
		tgt.Method = method
		tgt.URL = baseURL + path
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + apiKey},
			"Content-Type":  []string{"application/json"},
		}
		if bodyFn != nil {
			tgt.Body = bodyFn()
		}
		return nil
	}
}

// Data seeding (via HTTP to test server)

// seedJob creates a job and returns its ID.
func seedJob(t *testing.T, projectID string) string {
	t.Helper()
	slug := "load-" + newID()
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`,
		projectID, slug, slug, slug,
	)
	resp := httpDo(t, "POST", "/v1/jobs/", body, nil)
	return resp["id"].(string)
}

// seedJobWithTags creates a job with tags and returns its ID.
func seedJobWithTags(t *testing.T, projectID string, tags map[string]string) string {
	t.Helper()
	slug := "load-" + newID()
	tagsJSON, _ := json.Marshal(tags)
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"tags":%s}`,
		projectID, slug, slug, slug, tagsJSON,
	)
	resp := httpDo(t, "POST", "/v1/jobs/", body, nil)
	return resp["id"].(string)
}

// seedManyJobs creates n jobs and returns their IDs.
func seedManyJobs(t *testing.T, projectID string, n int) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range n {
		ids[i] = seedJob(t, projectID)
		if i > 0 && i%100 == 0 {
			t.Logf("seeded %d/%d jobs", i, n)
		}
	}
	return ids
}

type loadtestRunTokenClaims struct {
	Attempt int `json:"attempt,omitempty"`
	jwt.RegisteredClaims
}

// seedRun triggers a job and returns run ID plus an internal test SDK token.
func seedRun(t *testing.T, jobID string) (runID, runToken string) {
	t.Helper()
	body := `{"payload":{"load":"test"}}`
	resp := httpDo(t, "POST", "/v1/jobs/"+jobID+"/trigger", body, nil)
	runID, ok := resp["id"].(string)
	if !ok || runID == "" {
		t.Fatalf("trigger response missing id: %#v", resp)
	}
	return runID, mintRunToken(t, runID)
}

func mintRunToken(t *testing.T, runID string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, loadtestRunTokenClaims{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    domain.RunTokenIssuer,
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("mint run token: %v", err)
	}
	return signed
}

// seedManyRuns triggers a job n times and returns run IDs.
func seedManyRuns(t *testing.T, jobID string, n int) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range n {
		id, _ := seedRun(t, jobID)
		ids[i] = id
		if i > 0 && i%100 == 0 {
			t.Logf("seeded %d/%d runs", i, n)
		}
	}
	return ids
}

// seedRunTerminal creates a run and transitions it to a terminal state.
func seedRunTerminal(t *testing.T, jobID string, status domain.RunStatus) string {
	t.Helper()
	id, _ := seedRun(t, jobID)
	ctx := context.Background()

	err := testStore.UpdateRunStatus(ctx, id, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seedRunTerminal dequeued: %v", err)
	}

	err = testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
	if err != nil {
		t.Fatalf("seedRunTerminal executing: %v", err)
	}

	switch status {
	case domain.StatusCompleted:
		err = testStore.UpdateRunStatus(ctx, id, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
			"finished_at": time.Now().UTC(),
		})
	case domain.StatusFailed:
		err = testStore.UpdateRunStatus(ctx, id, domain.StatusExecuting, domain.StatusFailed, map[string]any{
			"finished_at": time.Now().UTC(),
			"error":       "load test failure",
		})
	default:
		t.Fatalf("seedRunTerminal: unsupported terminal status %s", status)
	}
	if err != nil {
		t.Fatalf("seedRunTerminal %s: %v", status, err)
	}
	return id
}

// seedWorkflow creates a workflow and returns its ID.
func seedWorkflow(t *testing.T, projectID string) string {
	t.Helper()
	slug := "wf-load-" + newID()
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-%s","slug":"%s","enabled":true}`,
		projectID, slug, slug,
	)
	resp := httpDo(t, "POST", "/v1/workflows/", body, nil)
	return resp["id"].(string)
}

func seedSecret(t *testing.T, projectID string) string {
	t.Helper()
	secretKey := "secret-" + newID()
	body := fmt.Sprintf(
		`{"project_id":"%s","secret_key":"%s","value":"val-%s"}`,
		projectID, secretKey, newID(),
	)
	resp := httpDo(t, "POST", "/v1/secrets/", body, nil)
	secretID, ok := resp["id"].(string)
	if !ok || secretID == "" {
		t.Fatalf("seedSecret: missing id in response: %v", resp)
	}
	return secretID
}

func seedWorkflowRun(t *testing.T, projectID string) string {
	t.Helper()
	workflowID := seedWorkflow(t, projectID)
	resp := httpDo(t, "POST", fmt.Sprintf("/v1/workflows/%s/trigger", workflowID), `{"payload":{}}`, nil)
	wfRunID, ok := resp["id"].(string)
	if !ok || wfRunID == "" {
		wfRunID, _ = resp["workflow_run_id"].(string)
	}
	if wfRunID == "" {
		t.Fatalf("seedWorkflowRun: no workflow_run_id in response: %v", resp)
	}
	return wfRunID
}

// seedRole creates a role and returns its ID.
func seedRole(t *testing.T) string {
	t.Helper()
	body := fmt.Sprintf(
		`{"name":"load-role-%s","description":"load test role","permissions":["jobs:read","runs:read","stats:read"]}`,
		newID(),
	)
	resp := httpDo(t, "POST", "/v1/roles", body, nil)
	return resp["id"].(string)
}

// seedAPIKey creates an API key and returns (keyID, rawKey).
func seedAPIKey(t *testing.T, projectID string, scopes []string) (keyID, rawKey string) {
	t.Helper()
	scopesJSON, _ := json.Marshal(scopes)
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-key-%s","scopes":%s,"expires_in_days":30}`,
		projectID, newID(), scopesJSON,
	)
	resp := httpDo(t, "POST", "/v1/api-keys/", body, nil)
	return resp["id"].(string), resp["key"].(string)
}

// seedEnvironment creates an environment and returns its ID.
func seedEnvironment(t *testing.T, projectID string) string {
	t.Helper()
	slug := "env-" + newID()
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-env-%s","slug":"%s","variables":{"KEY":"value","DB_HOST":"localhost"}}`,
		projectID, slug, slug,
	)
	resp := httpDo(t, "POST", "/v1/environments/", body, nil)
	return resp["id"].(string)
}

// seedJobGroup creates a job group and returns its ID.
func seedJobGroup(t *testing.T, projectID string) string {
	t.Helper()
	slug := "grp-" + newID()
	body := fmt.Sprintf(
		`{"project_id":"%s","name":"load-group-%s","slug":"%s"}`,
		projectID, slug, slug,
	)
	resp := httpDo(t, "POST", "/v1/job-groups/", body, nil)
	return resp["id"].(string)
}

// seedWebhookSubscription creates a webhook subscription and returns its ID.
func seedWebhookSubscription(t *testing.T, projectID string) string {
	t.Helper()
	body := fmt.Sprintf(
		`{"project_id":"%s","webhook_url":"https://example.com/webhook-%s","event_types":["run.completed","run.failed"],"secret":"whsec-%s"}`,
		projectID, newID(), newID(),
	)
	resp := httpDo(t, "POST", "/v1/webhooks/subscriptions/", body, nil)
	return resp["id"].(string)
}

// seedEventTrigger creates an event trigger in the store directly.
func seedEventTrigger(t *testing.T, projectID string) (triggerID, eventKey string) {
	t.Helper()
	triggerID = "evt-" + newID()
	eventKey = "load:" + newID()
	trigger := &domain.EventTrigger{
		ID:          triggerID,
		EventKey:    eventKey,
		ProjectID:   projectID,
		SourceType:  domain.EventSourceJobRun,
		TriggerType: "event",
		Status:      domain.EventTriggerStatusWaiting,
		TimeoutSecs: 3600,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := testStore.CreateEventTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("seedEventTrigger: %v", err)
	}
	return triggerID, eventKey
}

// seedMember creates a role and assigns a member, returning (roleID, userID).
func seedMember(t *testing.T) (roleID, userID string) {
	t.Helper()
	roleID = seedRole(t)
	userID = "user-" + newID()
	body := fmt.Sprintf(`{"user_id":"%s","role_id":"%s"}`, userID, roleID)
	httpDo(t, "POST", "/v1/members", body, nil)
	return roleID, userID
}

// HTTP helper

// httpDo performs an authenticated HTTP request and returns the decoded JSON body.
func httpDo(t *testing.T, method, path, body string, extraHeaders http.Header) map[string]any {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		t.Fatalf("httpDo new request: %v", err)
	}
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")
	maps.Copy(req.Header, extraHeaders)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("httpDo %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("httpDo %s %s: status %d body=%s", method, path, resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if len(respBody) > 0 {
		if jsonErr := json.Unmarshal(respBody, &result); jsonErr != nil {
			return map[string]any{}
		}
	}
	return result
}

// Utilities

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func mustClean(t *testing.T) {
	t.Helper()
	if err := testEnv.DB.CleanTables(context.Background()); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return fallback
}
