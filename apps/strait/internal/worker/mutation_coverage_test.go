package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMutationCoverage_AdaptiveConcurrencyQueueDepthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queueDepth int
		current    int
		want       int
	}{
		{name: "mild at one hundred", queueDepth: 100, current: 20, want: 25},
		{name: "moderate above one hundred", queueDepth: 101, current: 40, want: 60},
		{name: "moderate at one thousand", queueDepth: 1000, current: 40, want: 60},
		{name: "deep above one thousand", queueDepth: 1001, current: 40, want: 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := NewAdaptiveConcurrency(1, 200, tt.current)
			assert.Equal(t, tt.want, a.Observe(tt.queueDepth, 0.71))
		})
	}
}

func TestMutationCoverage_AdaptivePollDefaultsAndBounds(t *testing.T) {
	t.Parallel()

	defaults := NewAdaptivePollInterval(0, 0, 0)
	assert.Equal(t, 5*time.Second, defaults.Next())

	clamped := NewAdaptivePollInterval(time.Second, 30*time.Second, 200*time.Millisecond)
	clamped.ObserveDepth(1_000_000)
	assert.Equal(t, 200*time.Millisecond, clamped.Next())

	for range 20 {
		defaults.ObserveEmpty()
	}
	assert.Equal(t, 16, defaults.emptyCount)
}

func TestMutationCoverage_ResolveExecutorConfigHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defaultDegradedPollInterval, resolveDegradedPollInterval(0))
	assert.Equal(t, 15*time.Millisecond, resolveDegradedPollInterval(15*time.Millisecond))

	assert.Equal(t, time.Duration(-1), resolveTerminalRetryTimeout(-time.Second))
	assert.Equal(t, defaultTerminalRetryTimeout, resolveTerminalRetryTimeout(0))
	assert.Equal(t, time.Second, resolveTerminalRetryTimeout(time.Second))

	initial, maxDelay := resolveTerminalRetryBackoff(0, 0)
	assert.Equal(t, defaultTerminalRetryInitial, initial)
	assert.Equal(t, defaultTerminalRetryMax, maxDelay)

	initial, maxDelay = resolveTerminalRetryBackoff(2*time.Second, time.Second)
	assert.Equal(t, 2*time.Second, initial)
	assert.Equal(t, 2*time.Second, maxDelay)
}

func TestMutationCoverage_ResolveEventChannelSizeBoundaries(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defaultEventChannelSize, resolveEventChannelSize(0))
	assert.Equal(t, minEventChannelSize, resolveEventChannelSize(minEventChannelSize-1))
	assert.Equal(t, minEventChannelSize, resolveEventChannelSize(minEventChannelSize))
	assert.Equal(t, minEventChannelSize+1, resolveEventChannelSize(minEventChannelSize+1))
}

func TestMutationCoverage_ExecutorHTTPAndWebhookDefaults(t *testing.T) {
	t.Parallel()

	client := resolveExecutorHTTPClient(ExecutorConfig{
		ExecutorHTTPTimeout:     -time.Second,
		ExecutorIdleConnTimeout: -time.Second,
	})
	assert.Equal(t, defaultExecutorHTTPTimeout, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, defaultExecutorIdleConnTimeout, transport.IdleConnTimeout)

	webhookTransport := newSafeWebhookTransport()
	assert.Equal(t, webhookMaxIdleConns, webhookTransport.MaxIdleConns)
	assert.Equal(t, webhookMaxIdlePerHost, webhookTransport.MaxIdleConnsPerHost)
	assert.Equal(t, webhookIdleConnTimeout, webhookTransport.IdleConnTimeout)
	assert.Equal(t, webhookTimeout, webhookClient.Timeout)
}

func TestMutationCoverage_ClassifyErrorDirectBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: domain.ErrorClassUnknown},
		{name: "too many requests", err: &domain.EndpointError{StatusCode: http.StatusTooManyRequests}, want: domain.ErrorClassRateLimited},
		{name: "auth", err: &domain.EndpointError{StatusCode: http.StatusUnauthorized}, want: domain.ErrorClassAuth},
		{name: "client", err: &domain.EndpointError{StatusCode: http.StatusBadRequest}, want: domain.ErrorClassClient},
		{name: "server", err: &domain.EndpointError{StatusCode: http.StatusInternalServerError}, want: domain.ErrorClassServer},
		{name: "oom", err: errors.New("process hit ENOMEM"), want: domain.ErrorClassOOM},
		{name: "connection text", err: errors.New("broken pipe"), want: domain.ErrorClassConnection},
		{name: "op error", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("boom")}, want: domain.ErrorClassConnection},
		{name: "net error", err: temporaryNetError{}, want: domain.ErrorClassTransient},
		{name: "canceled", err: context.Canceled, want: domain.ErrorClassTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifyError(tt.err))
		})
	}
}

type temporaryNetError struct{}

func (temporaryNetError) Error() string   { return "temporary network failure" }
func (temporaryNetError) Timeout() bool   { return false }
func (temporaryNetError) Temporary() bool { return true }

func TestMutationCoverage_RetryablePostgresErrorBranches(t *testing.T) {
	t.Parallel()

	assert.False(t, isRetryablePostgresError(nil))
	assert.True(t, isRetryablePostgresError(context.DeadlineExceeded))
	assert.False(t, isRetryablePostgresError(errors.New("plain error")))
	assert.True(t, isRetryablePostgresError(&pgconn.PgError{Code: "25P02"}))
	assert.True(t, isRetryablePostgresError(&pgconn.PgError{Code: "08006"}))
	assert.False(t, isRetryablePostgresError(&pgconn.PgError{Code: "23505"}))
}

func TestMutationCoverage_RunSubscriptionHelpers(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{{
		ID:         "sub-1",
		EventTypes: []string{domain.WebhookEventRunCompleted},
	}}
	clone := cloneWebhookSubscriptions(subs)
	require.Len(t, clone, 1)
	clone[0].EventTypes[0] = domain.WebhookEventRunFailed
	assert.Equal(t, domain.WebhookEventRunCompleted, subs[0].EventTypes[0])
	assert.Nil(t, cloneWebhookSubscriptions(nil))

	for _, status := range []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusDeadLetter,
		domain.StatusTimedOut,
		domain.StatusCanceled,
		domain.StatusExpired,
	} {
		eventType, ok := runWebhookEventType(status)
		require.True(t, ok)
		assert.NotEmpty(t, eventType)
	}
	_, ok := runWebhookEventType(domain.StatusExecuting)
	assert.False(t, ok)

	assert.True(t, matchesRunSubscriptionEvent([]string{"*"}, domain.WebhookEventRunCompleted))
	assert.True(t, matchesRunSubscriptionEvent([]string{domain.WebhookEventRunCompleted}, domain.WebhookEventRunCompleted))
	assert.False(t, matchesRunSubscriptionEvent([]string{domain.WebhookEventRunFailed}, domain.WebhookEventRunCompleted))
}

func TestMutationCoverage_RunSubscriptionWebhookPayload(t *testing.T) {
	t.Parallel()

	payload, err := runSubscriptionWebhookPayload(&domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		Result:    []byte(`{"ok":true}`),
	}, domain.WebhookEventRunCompleted)
	require.NoError(t, err)

	assert.Contains(t, string(payload), `"type":"run.completed"`)
	assert.Contains(t, string(payload), `"attempt":2`)
}

func TestMutationCoverage_RunSubscriptionWebhookDeliveries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Result:    json.RawMessage(`{"ok":true}`),
	}
	subs := []domain.WebhookSubscription{
		{
			ID:         "sub-active",
			ProjectID:  "project-1",
			WebhookURL: "https://example.com/active",
			EventTypes: []string{domain.WebhookEventRunCompleted},
			Active:     true,
		},
		{
			ID:         "sub-inactive",
			ProjectID:  "project-1",
			WebhookURL: "https://example.com/inactive",
			EventTypes: []string{domain.WebhookEventRunCompleted},
			Active:     false,
		},
		{
			ID:         "sub-wrong-event",
			ProjectID:  "project-1",
			WebhookURL: "https://example.com/wrong",
			EventTypes: []string{domain.WebhookEventRunFailed},
			Active:     true,
		},
	}

	deliveries, err := buildRunSubscriptionWebhookDeliveries(
		run,
		domain.WebhookEventRunCompleted,
		subs,
		4,
		now,
	)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	delivery := deliveries[0]
	assert.Equal(t, "sub-active", delivery.SubscriptionID)
	assert.Equal(t, run.ID, delivery.RunID)
	assert.Equal(t, run.JobID, delivery.JobID)
	assert.Equal(t, 4, delivery.MaxAttempts)
	require.NotNil(t, delivery.NextRetryAt)
	assert.True(t, delivery.NextRetryAt.Equal(now.Add(-time.Second)))
	assert.Contains(t, string(delivery.Payload), `"type":"run.completed"`)
	assert.Equal(t, "webhook_subscriptions:project-1", runWebhookSubscriptionsCacheKey("project-1"))

	_, err = buildRunSubscriptionWebhookDeliveries(
		&domain.JobRun{ID: "run-bad-payload", Result: json.RawMessage(`{`)},
		domain.WebhookEventRunCompleted,
		subs,
		4,
		now,
	)
	require.Error(t, err)
}

func TestMutationCoverage_MappedPayloadFieldRawFallback(t *testing.T) {
	t.Parallel()

	rawValue := gjson.GetBytes([]byte(`{"name":"Ada"}`), "name")
	assert.Equal(t, `"Ada"`, string(mappedPayloadFieldRaw(rawValue)))

	rawEmptyValue := gjson.Result{Type: gjson.String, Str: "Ada"}
	assert.Equal(t, `"Ada"`, string(mappedPayloadFieldRaw(rawEmptyValue)))
}

func TestMutationCoverage_RetryTerminalCompletionNegativeTimeout(t *testing.T) {
	t.Parallel()

	exec := &Executor{terminalRetryTimeout: -1}
	attempts := 0
	err := exec.retryTerminalCompletion(
		context.Background(),
		&domain.JobRun{ID: "run-1"},
		&domain.Job{ID: "job-1"},
		func(context.Context) error {
			attempts++
			return retryableCompletionErr{}
		},
	)
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestMutationCoverage_SendWebhookOnceBuildFailures(t *testing.T) {
	t.Parallel()

	result := sendWebhookOnceWith(
		context.Background(),
		http.DefaultClient,
		&domain.Job{WebhookURL: "https://example.com/hook"},
		&domain.JobRun{ID: "run-1", Result: json.RawMessage(`{`)},
	)
	require.False(t, result.Delivered)
	assert.Contains(t, result.Error, "marshal failed")

	result = sendWebhookOnceWith(
		context.Background(),
		http.DefaultClient,
		&domain.Job{WebhookURL: "http://[::1"},
		&domain.JobRun{ID: "run-1"},
	)
	require.False(t, result.Delivered)
	assert.Contains(t, result.Error, "request build failed")
}

func TestMutationCoverage_ExecutionModeAndActorLabels(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", executionModeTier(RunLifecycleEvent{}))
	assert.Equal(t, "unknown", executionModeTier(RunLifecycleEvent{Job: &domain.Job{}}))
	assert.Equal(t, string(domain.ExecutionModeWorker), executionModeTier(RunLifecycleEvent{
		Job: &domain.Job{ExecutionMode: domain.ExecutionModeWorker},
	}))

	tests := []struct {
		name string
		run  *domain.JobRun
		want string
	}{
		{name: "nil", run: nil, want: ""},
		{name: "metadata override", run: &domain.JobRun{Metadata: map[string]string{domain.RunMetadataSentryActorType: "system"}}, want: "system"},
		{name: "api key", run: &domain.JobRun{CreatedBy: "apikey:abc"}, want: "api_key"},
		{name: "run token", run: &domain.JobRun{CreatedBy: "run:abc"}, want: "run_token"},
		{name: "sse token", run: &domain.JobRun{CreatedBy: "sse:abc"}, want: "sse_token"},
		{name: "user", run: &domain.JobRun{CreatedBy: "user-1"}, want: "user"},
		{name: "empty", run: &domain.JobRun{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, workerActorType(tt.run))
		})
	}
}

func TestMutationCoverage_MetadataCacheResetsWhenFull(t *testing.T) {
	t.Parallel()

	cache := newExecutorMetadataCache[int](time.Hour, nil)
	require.NotNil(t, cache)
	cache.mu.Lock()
	for i := range dispatchMetadataCacheMaxKeys {
		cache.entries[string(rune(i))] = executorMetadataCacheEntry[int]{
			value:     i,
			expiresAt: time.Now().Add(time.Hour),
		}
	}
	cache.mu.Unlock()

	cache.Set("fresh", 42)
	got, ok := cache.Get("fresh")
	require.True(t, ok)
	assert.Equal(t, 42, got)
	assert.Len(t, cache.entries, 1)
}

func TestMutationCoverage_WorkflowStepsTierConfigDisablesL2WhenNil(t *testing.T) {
	t.Parallel()

	cfg := workerWorkflowStepsTierConfig(time.Minute, nil)
	assert.True(t, cfg.DisableL2)
	assert.Equal(t, uint32(1), cfg.Weigher(workflowStepsVersionKey{}, nil))
	assert.Equal(t, uint32(100_000), cfg.Weigher(workflowStepsVersionKey{}, make([]domain.WorkflowStep, 100_001)))
	assert.Equal(t, uint32(2), cfg.Weigher(workflowStepsVersionKey{}, make([]domain.WorkflowStep, 2)))
}

func TestMutationCoverage_ComputeAvailableAdaptiveAndBatchBounds(t *testing.T) {
	t.Parallel()

	pool := NewPool(2)
	t.Cleanup(func() {
		require.NoError(t, pool.Shutdown(context.Background()))
	})
	block := make(chan struct{})
	started := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-block
	})
	<-started
	t.Cleanup(func() { close(block) })

	exec := &Executor{
		pool:                pool,
		concurrencyLimit:    NewAdaptiveConcurrency(1, 10, 1),
		maxDequeueBatchSize: 10,
	}
	assert.Equal(t, 0, exec.computeAvailable())

	exec.concurrencyLimit = NewAdaptiveConcurrency(1, 10, 10)
	exec.maxDequeueBatchSize = 1
	assert.Equal(t, 1, exec.computeAvailable())
}

type alwaysFailingHeartbeatStore struct{}

func (alwaysFailingHeartbeatStore) UpdateHeartbeat(context.Context, string) error {
	return errors.New("heartbeat failed")
}

func (alwaysFailingHeartbeatStore) BatchUpdateHeartbeat(context.Context, []string) error {
	return errors.New("heartbeat failed")
}

func TestMutationCoverage_HeartbeatFlushFailureThreshold(t *testing.T) {
	t.Parallel()

	manager := NewHeartbeatManager(alwaysFailingHeartbeatStore{}, time.Hour)
	manager.Register("run-1")
	for range 3 {
		manager.flush(context.Background())
	}
	assert.Equal(t, 3, manager.consecutiveFailures)
}

type failingLegacyHeartbeatStore struct {
	calls []string
}

func (s *failingLegacyHeartbeatStore) UpdateHeartbeat(_ context.Context, id string) error {
	s.calls = append(s.calls, id)
	if id == "bad" {
		return errors.New("heartbeat failed")
	}
	return nil
}

func TestMutationCoverage_HeartbeatLegacyAdapterStopsOnError(t *testing.T) {
	t.Parallel()

	store := &failingLegacyHeartbeatStore{}
	adapter := heartbeatStoreAdapter{store: store}
	err := adapter.BatchUpdateHeartbeat(context.Background(), []string{"ok", "bad", "after"})
	require.Error(t, err)
	assert.Equal(t, []string{"ok", "bad"}, store.calls)
}
