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
		if !strings.Contains(logs, field) {
			t.Fatalf("expected logs to contain field %q, got: %s", field, logs)
		}
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
		if !strings.Contains(logs, field) {
			t.Fatalf("expected profiling log to contain field %q, got: %s", field, logs)
		}
	}
	if strings.Contains(logs, "pprof-secret-value") {
		t.Fatalf("profiling secret leaked in startup log: %s", logs)
	}
}

func TestProfilingManagementAddr(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		ProfilingManagementBindAddr: "::1",
		ProfilingManagementPort:     18080,
	}
	if got := profilingManagementAddr(cfg); got != "[::1]:18080" {
		t.Fatalf("profilingManagementAddr() = %q, want [::1]:18080", got)
	}
}

func TestShutdownReason(t *testing.T) {
	t.Helper()

	if got := shutdownReason(nil); got != "graceful" {
		t.Fatalf("shutdownReason(nil) = %q, want graceful", got)
	}
	if got := shutdownReason(context.DeadlineExceeded); got != "timeout" {
		t.Fatalf("shutdownReason(DeadlineExceeded) = %q, want timeout", got)
	}
	if got := shutdownReason(errors.New("forced")); got != "forced" {
		t.Fatalf("shutdownReason(other error) = %q, want forced", got)
	}
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
	if got := primary["workflow_step_runs"]; got != 0 {
		t.Fatalf("workflow_step_runs primary fanout handlers = %d, want 0", got)
	}

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
		if got := notificationWorkerEnabled(tt.mode); got != tt.want {
			t.Fatalf("notificationWorkerEnabled(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestStartGRPCServer_RequiresPubsubWhenEnabled(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		Mode:        "api",
		GRPCEnabled: true,
	}

	srv, err := startGRPCServer(pool.New().WithContext(context.Background()), cfg, nil, nil, nil, nil, nil, "test", nil)
	if err == nil {
		t.Fatal("expected error when GRPC is enabled without pubsub")
	}
	if srv != nil {
		t.Fatal("expected nil grpc server when startup fails")
	}
	if !strings.Contains(err.Error(), "no pubsub publisher is configured") {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err := waitForPubsubReady(context.Background(), pub, time.Second); err != nil {
		t.Fatalf("waitForPubsubReady() error = %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("ping calls = %d, want 3", got)
	}
}

func TestWaitForPubsubReady_TimesOut(t *testing.T) {
	t.Helper()

	pub := flakyPingPub{pingFn: func(context.Context) error { return errors.New("redis down") }}
	err := waitForPubsubReady(context.Background(), pub, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "pubsub readiness timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartGRPCServer_DisabledReturnsNil(t *testing.T) {
	t.Helper()

	cfg := &config.Config{
		Mode:        "all",
		GRPCEnabled: false,
	}

	srv, err := startGRPCServer(pool.New().WithContext(context.Background()), cfg, nil, nil, nil, nil, nil, "test", nil)
	if err != nil {
		t.Fatalf("startGRPCServer() error = %v", err)
	}
	if srv != nil {
		t.Fatal("expected nil grpc server when GRPC is disabled")
	}
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
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer plane.GracefulStop()

	execCfg := workerExecutorConfigForTest()
	applyWorkerPlaneToExecutorConfig(&execCfg, plane, "jwt-signing-key")

	if execCfg.QueueSnapshotter == nil {
		t.Fatal("QueueSnapshotter was not wired")
	}
	if execCfg.WorkerDispatcher == nil {
		t.Fatal("WorkerDispatcher was not wired")
	}
	if got := execCfg.QueueSnapshotter.SnapshotWorkerQueues(); got != nil {
		t.Fatalf("SnapshotWorkerQueues on empty registry = %v, want nil", got)
	}
}

func TestApplyWorkerPlaneToExecutorConfig_NilPlaneLeavesConfigUntouched(t *testing.T) {
	t.Helper()

	execCfg := workerExecutorConfigForTest()
	applyWorkerPlaneToExecutorConfig(&execCfg, nil, "jwt-signing-key")

	if execCfg.QueueSnapshotter != nil {
		t.Fatal("QueueSnapshotter should remain nil without worker plane")
	}
	if execCfg.WorkerDispatcher != nil {
		t.Fatal("WorkerDispatcher should remain nil without worker plane")
	}
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
	if got := counts[table]; got < minCount {
		t.Fatalf("%s handlers = %d, want at least %d", table, got, minCount)
	}
}
