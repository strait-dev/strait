package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	grpcserver "strait/internal/api/grpc"
	straitcache "strait/internal/cache"
	"strait/internal/cdc"
	"strait/internal/clickhouse"
	"strait/internal/config"
	"strait/internal/pubsub"
	"strait/internal/scheduler"
	"strait/internal/worker"

	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/sourcegraph/conc/pool"
	"github.com/stretchr/testify/require"
)

func TestWorkerShutdownTelemetryLogsContainExpectedFields(t *testing.T) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	startedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	logWorkerShutdownStart(logger, startedAt, 3, 15*time.Second)
	logWorkerShutdownComplete(logger, nil, startedAt.Add(4*time.Second), 2, "graceful", nil)

	logs := buf.String()
	for _, field := range []string{
		"shutdown_started_at",
		"in_flight_runs",
		"drain_timeout",
		"shutdown_completed_at",
		"runs_drained",
	} {
		require.Contains(t, logs, field)
	}
}

func TestProfilingStartupLogDoesNotLeakSecrets(t *testing.T) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	cfg := &config.Config{
		ProfilingEnabled:            true,
		ProfilingAPIEnabled:         false,
		ProfilingManagementEnabled:  true,
		ProfilingManagementBindAddr: "127.0.0.1",
		ProfilingManagementPort:     18080,
		ProfilingMutexFraction:      50,
		ProfilingBlockRate:          250000,
		ProfilingSecret:             "pprof-secret-value",
		ProfilingAllowedCIDRs:       []string{"127.0.0.1/32"},
	}

	logProfilingStartup(logger, cfg)

	logs := buf.String()
	for _, field := range []string{
		"profiling_secret_configured",
		"cidr_allowlist_configured",
		"api_listener",
		"management_listener",
		"mutex_fraction",
		"block_rate",
		"cpu_profile_max_seconds",
		"management_bind_addr",
	} {
		require.Contains(t, logs, field)
	}
	require.NotContains(t, logs, "pprof-secret-value")
}

func TestProfilingManagementAddr(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		ProfilingManagementBindAddr: "::1",
		ProfilingManagementPort:     18080,
	}
	require.Equal(t, "[::1]:18080",
		profilingManagementAddr(
			cfg))
}

func TestShutdownReason(t *testing.T) {
	t.Helper()
	require.Equal(t, "graceful",
		shutdownReason(nil))
	require.Equal(t, "timeout",
		shutdownReason(context.
			DeadlineExceeded,
		))
	require.Equal(t, "forced",
		shutdownReason(errors.New("forced")))
}

func TestRegisterCDCDeliveryHandlers_WiresLaunchCDCTables(t *testing.T) {
	t.Helper()

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = rdb.Close() })
	cacheHandlers := cdc.NewCacheReadModelHandlers(rdb, time.Minute, nil)
	cacheBus := straitcache.NewBus(noopServicePub{}, straitcache.BusConfig{Origin: "test"})
	registrar := &recordingCDCRegistrar{}

	registerCDCDeliveryHandlers(registrar, noopServicePub{}, nil, &clickhouse.Exporter{}, cacheHandlers, cacheBus, nil)

	primary := tableCounts(registrar.primary)
	requireTableCount(t, primary, "job_runs", 1)
	requireTableCount(t, primary, "workflow_runs", 1)
	requireTableCount(t, primary, "event_triggers", 1)
	require.Equal(t, 0, primary["workflow_step_runs"])

	additional := tableCounts(registrar.additional)
	requireTableCount(t, additional, "job_runs", 4)
	requireTableCount(t, additional, "workflow_runs", 1)
	requireTableCount(t, additional, "workflow_step_runs", 1)
	for _, table := range []string{
		"api_keys",
		"project_roles",
		"project_member_roles",
		"resource_policies",
		"tag_policies",
		"project_quotas",
		"organization_subscriptions",
		"jobs",
		"job_dependencies",
	} {
		requireTableCount(t, additional, table, 1)
	}

	total := tableCounts(append(append([]string{}, registrar.primary...), registrar.additional...))
	for _, table := range cdc.RequiredConsumerTables() {
		table = strings.TrimPrefix(table, "public.")
		requireTableCount(t, total, table, 1)
	}
}

func TestNotificationWorkerEnabled(t *testing.T) {
	t.Helper()

	tests := []struct {
		mode string
		want bool
	}{
		{mode: "api", want: false},
		{mode: "worker", want: true},
		{mode: "all", want: true},
		{mode: "", want: false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want,
			notificationWorkerEnabled(tt.mode))
	}
}

func TestStartGRPCServer_RequiresPubsubWhenEnabled(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		Mode:        "api",
		GRPCEnabled: true,
	}

	srv, err := startGRPCServer(pool.New().WithContext(context.Background()), cfg, nil, nil, nil, nil, nil, "test", nil)
	require.Error(t, err)
	require.Nil(t, srv)
	require.Contains(t, err.Error(), "no pubsub publisher is configured")
}

func TestWaitForPubsubReady_RetriesUntilHealthy(t *testing.T) {
	t.Helper()

	var calls atomic.Int32
	pub := flakyPingPub{
		pingFn: func(context.Context) error {
			if calls.Add(1) < 3 {
				return errors.New("redis warming up")
			}
			return nil
		},
	}
	require.NoError(t, waitForPubsubReady(context.
		Background(), pub, time.Second,
	))
	require.EqualValues(t, 3, calls.
		Load())
}

func TestWaitForPubsubReady_TimesOut(t *testing.T) {
	t.Helper()

	pub := flakyPingPub{pingFn: func(context.Context) error { return errors.New("redis down") }}
	err := waitForPubsubReady(context.Background(), pub, 20*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pubsub readiness timeout")
}

func TestStartGRPCServer_DisabledReturnsNil(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		Mode:        "all",
		GRPCEnabled: false,
	}

	srv, err := startGRPCServer(pool.New().WithContext(context.Background()), cfg, nil, nil, nil, nil, nil, "test", nil)
	require.NoError(t, err)
	require.Nil(t, srv)
}

func TestApplyWorkerPlaneToExecutorConfig_WiresDispatcherAndSnapshotter(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		GRPCEnabled:          true,
		GRPCPort:             0,
		GRPCKeepaliveTime:    30 * time.Second,
		GRPCKeepaliveTimeout: 10 * time.Second,
	}
	plane, err := grpcserver.NewServer(cfg, nil, noopServicePub{})
	require.NoError(t, err)

	defer plane.GracefulStop()

	execCfg := workerExecutorConfigForTest()
	applyWorkerPlaneToExecutorConfig(&execCfg, plane, "jwt-signing-key")
	require.NotNil(t, execCfg.
		QueueSnapshotter,
	)
	require.NotNil(t, execCfg.
		WorkerDispatcher,
	)
	require.Nil(t, execCfg.
		QueueSnapshotter.
		SnapshotWorkerQueues())
}

func TestApplyWorkerPlaneToExecutorConfig_NilPlaneLeavesConfigUntouched(t *testing.T) {
	t.Helper()

	execCfg := workerExecutorConfigForTest()
	applyWorkerPlaneToExecutorConfig(&execCfg, nil, "jwt-signing-key")
	require.Nil(t, execCfg.
		QueueSnapshotter,
	)
	require.Nil(t, execCfg.
		WorkerDispatcher,
	)
}

func workerExecutorConfigForTest() worker.ExecutorConfig {
	return worker.ExecutorConfig{}
}

// TestAnomalyMonitorStore_SatisfiesInterface fails to build if the wrapper
// drifts from scheduler.AnomalyMonitorStore. The runtime scheduler is built
// with a non-nil anomaly monitor, and this compile-time check keeps the wrapper
// aligned as the interface evolves.
func TestAnomalyMonitorStore_SatisfiesInterface(t *testing.T) {
	t.Helper()
	var _ scheduler.AnomalyMonitorStore = (*anomalyMonitorStore)(nil)
}

type noopServicePub struct{}

func (noopServicePub) Ping(context.Context) error {
	return nil
}

func (noopServicePub) Publish(context.Context, string, []byte) error {
	return nil
}

func (noopServicePub) PublishBatch(context.Context, []pubsub.PubSubMessage) error {
	return nil
}

func (noopServicePub) Subscribe(ctx context.Context, _ string) (*pubsub.Subscription, error) {
	var concWG conc.WaitGroup
	ch := make(chan []byte)
	subCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		<-subCtx.Done()
		close(ch)
	})
	return pubsub.NewSubscription(ch, func() {
		cancel()
		concWG.Wait()
	}), nil
}

func (noopServicePub) Close() error {
	return nil
}

type flakyPingPub struct {
	noopServicePub
	pingFn func(context.Context) error
}

func (p flakyPingPub) Ping(ctx context.Context) error {
	if p.pingFn != nil {
		return p.pingFn(ctx)
	}
	return nil
}

type recordingCDCRegistrar struct {
	primary    []string
	additional []string
}

func (r *recordingCDCRegistrar) RegisterHandler(h cdc.Handler) {
	if h != nil {
		r.primary = append(r.primary, h.Table())
	}
}

func (r *recordingCDCRegistrar) RegisterAdditionalHandler(h cdc.Handler) {
	if h != nil {
		r.additional = append(r.additional, h.Table())
	}
}

func tableCounts(tables []string) map[string]int {
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		counts[table]++
	}
	return counts
}

func requireTableCount(t *testing.T, counts map[string]int, table string, minCount int) {
	t.Helper()
	require.GreaterOrEqual(
		t, counts[table], minCount,
	)
}
