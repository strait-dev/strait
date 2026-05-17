package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strait/internal/api"
	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/config"
	"strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/webhook"
	"strait/internal/workflow"

	"github.com/lmittmann/tint"
	"github.com/redis/go-redis/v9"
	concpool "github.com/sourcegraph/conc/pool"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

var version = "dev"
var commit = "none"
var date = "unknown"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	code := run(ctx)
	cancel()
	os.Exit(code)
}

func run(ctx context.Context) int {
	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runServe(ctx context.Context, modeOverride string) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// CLI flag overrides env
	if modeOverride != "" {
		cfg.Mode = modeOverride
	}

	// Validate mode
	switch cfg.Mode {
	case "api", "worker", "all":
		// Standard modes — fall through to normal service startup.
	default:
		return fmt.Errorf("invalid mode %q: must be api, worker, or all", cfg.Mode)
	}

	setupLogging(cfg.LogLevel, cfg.LogFormat)

	if cfg.SentryDSN != "" {
		release := cfg.SentryRelease
		if release == "" {
			release = telemetry.BuildSentryRelease(version, commit)
		}
		shutdownSentry, err := telemetry.InitSentry(telemetry.SentryConfig{
			DSN:                     cfg.SentryDSN,
			Environment:             cfg.SentryEnvironment,
			Release:                 release,
			TracesSampleRate:        cfg.SentryTracesSampleRate,
			Debug:                   cfg.SentryDebug,
			MaxBreadcrumbs:          cfg.SentryMaxBreadcrumbs,
			MaxSpans:                cfg.SentryMaxSpans,
			MaxErrorDepth:           cfg.SentryMaxErrorDepth,
			StrictTraceContinuation: cfg.SentryStrictTraceContinuation,
		})
		if err != nil {
			return err
		}
		defer shutdownSentry()

		// Re-wrap the slog handler to pipe Error-level logs to Sentry.
		currentHandler := slog.Default().Handler()
		sentryLogger := slog.New(telemetry.NewSentryHandler(currentHandler))
		slog.SetDefault(sentryLogger)

		slog.Info("sentry initialized",
			"environment", cfg.SentryEnvironment,
			"release", release,
			"traces_sample_rate", cfg.SentryTracesSampleRate,
		)
	}

	slog.Info("starting strait",
		"version", version,
		"commit", commit,
		"mode", cfg.Mode,
		"port", cfg.Port,
	)

	// Initialize OpenTelemetry tracing
	shutdownTracer, err := telemetry.Init(ctx, "strait", cfg.OTELEndpoint, cfg.SentryEnvironment)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownTracer(shutdownCtx); err != nil {
			slog.Error("failed to shutdown tracer", "error", err)
		}
	}()

	metrics, metricsHandler, shutdownMetrics, err := telemetry.InitMetrics("strait", cfg.SentryEnvironment)
	if err != nil {
		return fmt.Errorf("init metrics: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownMetrics(shutdownCtx); err != nil {
			slog.Error("failed to shutdown metrics", "error", err)
		}
	}()

	profilingShutdown, err := telemetry.InitProfiling(telemetry.ProfilingConfig{
		Endpoint:    cfg.PyroscopeEndpoint,
		AuthToken:   cfg.PyroscopeAuthToken,
		ServiceName: "strait",
		Environment: cfg.SentryEnvironment,
	})
	if err != nil {
		slog.Error("failed to init profiling", "error", err)
	} else {
		defer profilingShutdown()
	}

	// Initialize OTel log bridge to export structured logs via OTLP.
	otelLogger, shutdownLogBridge, err := telemetry.InitLogBridge(ctx, "strait", cfg.OTELEndpoint, cfg.SentryEnvironment)
	if err != nil {
		return fmt.Errorf("init log bridge: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownLogBridge(shutdownCtx); err != nil {
			slog.Error("failed to shutdown log bridge", "error", err)
		}
	}()
	if otelLogger != nil {
		// Chain OTel log handler with the existing handler (which may already
		// include Sentry) so log records flow to both stdout and the OTel pipeline.
		currentHandler := slog.Default().Handler()
		tee := telemetry.NewTeeHandler(currentHandler, otelLogger.Handler())
		slog.SetDefault(slog.New(tee))
	}

	// Initialize ClickHouse (optional analytics backend).
	// ClickHouse is never required for operational correctness — degrade gracefully.
	chClient, err := clickhouse.New(clickhouse.Config{
		URL:          cfg.ClickHouseURL,
		Database:     cfg.ClickHouseDatabase,
		Enabled:      cfg.ClickHouseEnabled,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}, slog.Default())
	if err != nil {
		slog.Warn("clickhouse unavailable, analytics disabled", "error", err)
	}
	if chClient != nil {
		if err := clickhouse.CreateSchema(ctx, chClient); err != nil {
			slog.Warn("clickhouse schema creation failed, analytics disabled", "error", err)
			_ = chClient.Close()
			chClient = nil
		}
	}
	if chClient != nil {
		slog.Info("clickhouse enabled", "database", cfg.ClickHouseDatabase)
	}

	// DB pool must be deferred before the exporter so LIFO order ensures:
	// exporter.Stop() (flush + goroutine drain) runs before dbPool.Close().
	dbPool, err := connectDatabase(ctx, cfg)
	if err != nil {
		if chClient != nil {
			_ = chClient.Close()
		}
		return err
	}
	defer dbPool.Close()
	if chClient != nil {
		defer chClient.Close()
	}

	chExporter := clickhouse.NewExporter(chClient, clickhouse.ExporterConfig{
		BatchSize:     cfg.ClickHouseBatchSize,
		FlushInterval: cfg.ClickHouseFlushInterval,
		Enabled:       cfg.ClickHouseExportEnabled,
	}, slog.Default())
	if chExporter != nil {
		if metrics != nil {
			chExporter.WithMetrics(&clickhouse.ExporterMetrics{
				DroppedRecords: metrics.ClickHouseDroppedRecords,
				FlushFailures:  metrics.ClickHouseFlushFailures,
			})
		}
		chExporter.Start(ctx)
		defer chExporter.Stop()
		slog.Info("clickhouse exporter enabled",
			"batch_size", cfg.ClickHouseBatchSize,
			"flush_interval", cfg.ClickHouseFlushInterval,
		)

		// Wire approval change hook so workflow approvals flow to ClickHouse.
		store.OnApprovalChanged = func(_ context.Context, approval *domain.WorkflowStepApproval) {
			if approval == nil {
				return
			}
			chExporter.Enqueue(clickhouse.WorkflowApprovalEventRecord{
				ApprovalID:    approval.ID,
				WorkflowRunID: approval.WorkflowRunID,
				StepRunID:     approval.WorkflowStepRunID,
				Status:        approval.Status,
				RequestedAt:   approval.RequestedAt,
				ApprovedAt:    approval.ApprovedAt,
			})
		}
	}

	poolTuner, err := store.NewPoolTuner(dbPool, slog.Default(), cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return fmt.Errorf("init pool tuner: %w", err)
	}

	// Run migrations
	if err := runMigrations(cfg.DatabaseURL, cfg.MigrationMode, cfg.MigrationLockTimeout); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	cacheWarmer, err := store.NewCacheWarmer(dbPool, slog.Default())
	if err != nil {
		return fmt.Errorf("init cache warmer: %w", err)
	}
	if err := cacheWarmer.Warm(ctx); err != nil {
		return fmt.Errorf("warm query cache: %w", err)
	}

	// Create dependencies.
	//
	// NewWithContextRouting wraps the pool in a DBTX that transparently
	// routes through a per-request transaction when one is bound to the
	// request context (see store.ContextWithTx and the rlsTxMiddleware
	// in internal/api). Worker-mode code paths never bind a tx so the
	// wrapper falls through to the pool — zero behavior change there.
	// The API path gets real RLS enforcement because the middleware
	// begins a tx, runs SELECT set_config('app.current_project_id', ...)
	// on it, and every subsequent store call inside the request runs on
	// the same tx.
	queries := store.NewWithContextRouting(dbPool)
	if chClient != nil {
		queries.SetClickHouseDB(chClient.DB())
	}
	queries.SetSecretEncryptionKey(cfg.SecretEncryptionKey)
	queries.SetOldSecretEncryptionKeys(cfg.EncryptionKeyOld)
	if cfg.RunRetentionShort > 0 {
		queries.SetMaxSLOWindowHours(int(cfg.RunRetentionShort.Hours()))
	}
	if cfg.InternalSecret != "" {
		auditKey, auditKeyErr := store.DeriveAuditSigningKey(cfg.InternalSecret)
		if auditKeyErr != nil {
			return fmt.Errorf("derive audit signing key: %w", auditKeyErr)
		}
		queries.SetAuditSigningKey(auditKey)
	}
	bp := queue.NewBackpressure(dbPool, queue.BackpressureConfig{}, true)
	q := queue.NewPostgresQueue(
		dbPool,
		queue.WithPriorityAging(true),
		queue.WithBackpressureController(bp),
	)

	pub, rdb, err := connectRedis(ctx, cfg)
	if err != nil {
		return err
	}
	if err := validateBillingRedisDependency(cfg, rdb); err != nil {
		return err
	}
	if rdb != nil {
		defer rdb.Close()
	}

	g := concpool.New().WithContext(ctx).WithFailFast()
	g.Go(func(ctx context.Context) error {
		return poolTuner.Run(ctx)
	})

	webhookOptions := []webhook.DeliveryWorkerOption{}
	if rdb != nil {
		webhookOptions = append(webhookOptions, webhook.WithCircuitBreaker(webhook.NewRedisWebhookCircuitBreaker(rdb, true)))
	}
	webhookOptions = append(webhookOptions,
		webhook.WithMetrics(metrics),
		webhook.WithMaxPayloadBytes(cfg.WebhookMaxPayloadBytes),
		webhook.WithConcurrency(cfg.WebhookConcurrency),
		webhook.WithChExporter(chExporter),
		webhook.WithAllowPrivateEndpoints(cfg.AllowPrivateEndpoints),
		webhook.WithHTTPTransport(cfg.WebhookTimeout, cfg.WebhookIdleConnTimeout, cfg.WebhookMaxIdleConns, cfg.WebhookMaxIdleConnsPerHost),
		webhook.WithBatchByURL(cfg.WebhookBatchEnabled),
		webhook.WithMaxBatchSize(cfg.WebhookMaxBatchSize),
	)
	// Webhook signing secrets are stored encrypted at rest when an
	// encryption key is configured, so the delivery worker must decrypt
	// before computing the outbound HMAC. Without this the signature is
	// computed over the AES-GCM ciphertext and cannot be verified by
	// subscribers.
	if webhookEnc, encErr := crypto.NewKeyRotatorFromStrings(cfg.EncryptionKey, cfg.EncryptionKeyOld...); cfg.EncryptionKey != "" && encErr == nil {
		webhookOptions = append(webhookOptions, webhook.WithSecretDecryptor(webhookEnc))
	} else if cfg.EncryptionKey != "" && encErr != nil {
		slog.Warn("failed to create encryptor for webhook secrets; webhook signature decryption disabled", "error", encErr)
	}
	eventNotifier := webhook.NewEventNotifier(queries, slog.Default(), webhookOptions...)

	onTriggerCreate := func(trigger *domain.EventTrigger) {
		if metrics != nil {
			attrs := otelmetric.WithAttributes(
				otelattr.String("source_type", trigger.SourceType),
				otelattr.String("project_id", trigger.ProjectID),
				otelattr.String("trigger_type", trigger.TriggerType),
			)
			metrics.EventTriggersCreated.Add(context.Background(), 1, attrs)
		}
		if chExporter != nil {
			chExporter.Enqueue(clickhouse.EventTriggerEventRecord{
				TriggerID:   trigger.ID,
				EventKey:    trigger.EventKey,
				ProjectID:   trigger.ProjectID,
				SourceType:  trigger.SourceType,
				Status:      domain.EventTriggerStatusWaiting,
				TimeoutSecs: uint32(max(trigger.TimeoutSecs, 0)), //nolint:gosec // timeout is always non-negative
				CreatedAt:   trigger.RequestedAt,
			})
		}
		eventNotifier.NotifyAsyncWithContext(ctx, trigger)
	}

	workflowEngine := workflow.NewWorkflowEngine(queries, q, slog.Default()).
		WithMaxNestingDepth(cfg.MaxWorkflowNestingDepth).
		WithMetrics(metrics).
		WithOnTriggerCreate(onTriggerCreate)
	stepCallback := workflow.NewStepCallback(queries, workflowEngine, slog.Default()).WithMetrics(metrics).WithChExporter(chExporter)

	healthReg := health.NewRegistry()
	healthReg.Register(health.NewChecker("database", func(ctx context.Context) error {
		_, err := queries.QueueStats(ctx)
		return err
	}))
	if rdb != nil {
		healthReg.Register(health.NewRedisChecker(redisPingerAdapter{rdb}))
	}
	healthReg.Register(health.NewAuditProbe(queries))
	healthReg.Register(health.NewAuditDMLGuardProbe(queries))
	logAuditDMLGuardStartup(ctx, queries, metrics)

	var apiEncryptor api.Encryptor
	if cfg.EncryptionKey != "" {
		enc, encErr := crypto.NewKeyRotatorFromStrings(cfg.EncryptionKey, cfg.EncryptionKeyOld...)
		if encErr != nil {
			slog.Warn("failed to create encryptor for API; event source signature encryption disabled", "error", encErr)
		} else {
			apiEncryptor = enc
		}
	}

	// Create a shared billing enforcer (used by both API webhook handler and worker executor).
	// Only created when billing enforcement is enabled or Stripe webhook secret is set.
	var billingEnforcer *billing.Enforcer
	var billingDispatcher *webhook.BillingDispatcher
	if rdb != nil && (cfg.BillingEnforcementEnabled || cfg.StripeWebhookSecret != "") {
		billingStore := billing.NewPgStore(dbPool)

		// The dispatcher fans org-scoped billing events out to project-level
		// webhook subscriptions. Constructed before NewEnforcer so it can be
		// plumbed into both the API enforcer and the worker-side schedulers
		// (Dunner / DowngradeApplier / SLACalculator).
		billingDispatcher = webhook.NewBillingDispatcher(eventNotifier, billingStore, queries, slog.Default())

		var enforcerOpts []billing.EnforcerOption
		billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default())
		if billingEmailSender != nil {
			enforcerOpts = append(enforcerOpts, billing.WithEnforcerBillingEmails(billingEmailSender))
		}
		enforcerOpts = append(enforcerOpts, billing.WithSentryRuntime(cfg.Mode, cfg.DefaultRegion, version))
		enforcerOpts = append(enforcerOpts, billing.WithEntitlementsAuthoritative(cfg.BillingEntitlementsAuthoritative))
		enforcerOpts = append(enforcerOpts, billing.WithBillingDispatcher(billingDispatcher))
		billingEnforcer = billing.NewEnforcer(billingStore, rdb, slog.Default(), enforcerOpts...)
		billingEnforcer.StartCleanup(ctx)

		// Wire webhook delivery cost recording. Each successful outbound delivery
		// is billed at the same flat rate as an HTTP run (20 micro-USD).
		webhookCostRecorder := billing.NewRunCostRecorder(billingStore, rdb, nil, slog.Default())
		eventNotifier.SetRunCostRecorder(webhookCostRecorder)
	}

	if (cfg.BillingEnforcementEnabled || cfg.StripeWebhookSecret != "") && cfg.StripeWebhookSecret == "" {
		slog.Warn("STRIPE_WEBHOOK_SECRET is empty -- Stripe webhook signature verification is DISABLED")
	}

	// R4 hardening: startup safety checks. These are the "fail loud"
	// mechanisms that prevent silent corruption.
	if err := scheduler.EnsureQueueTriggersPresent(ctx, dbPool); err != nil {
		return fmt.Errorf("queue trigger check: %w", err)
	}
	if err := queries.CheckSchemaVersion(ctx, domain.ExpectedSchemaVersion); err != nil {
		return fmt.Errorf("schema version: %w", err)
	}

	cdcWebhookReceiver := startCDCConsumer(g, cfg, pub, queries, chExporter)
	startWebhookWorker(g, cfg, eventNotifier)
	startNotificationWorker(g, cfg, queries, metrics)
	startLogDrainWorker(g, cfg, queries, metrics)
	startMaintenanceWorker(g, queries)
	startDBWatchdog(g, cfg, dbPool)
	startQueueHealthSampler(g, dbPool)
	startDBPoolSampler(g, dbPool)
	var chAnalytics api.AnalyticsStore
	if chClient != nil {
		chAnalytics = clickhouse.NewAnalyticsStore(chClient, clickhouse.NewPgHealthAdapter(dbPool))
	}
	workerPlane, err := startGRPCServer(g, cfg, queries, pub, rdb, billingEnforcer, version, apiEncryptor)
	if err != nil {
		return fmt.Errorf("starting grpc server: %w", err)
	}
	startAPIServer(g, cfg, queries, dbPool, dbPool, q, pub, metricsHandler, metrics, stepCallback, workflowEngine, healthReg, rdb, apiEncryptor, billingEnforcer, chAnalytics, chExporter, cdcWebhookReceiver)
	startWorker(g, cfg, queries, dbPool, dbPool, q, bp, pub, metrics, stepCallback, workflowEngine, healthReg, billingEnforcer, billingDispatcher, chExporter, workerPlane, apiEncryptor)

	if err := g.Wait(); err != nil {
		return fmt.Errorf("services: %w", err)
	}

	slog.Info("strait stopped")
	return nil
}

func validateBillingRedisDependency(cfg *config.Config, rdb *redis.Client) error {
	if cfg != nil && cfg.BillingEnforcementEnabled && rdb == nil {
		return fmt.Errorf("billing enforcement requires Redis; set REDIS_URL or disable BILLING_ENFORCEMENT_ENABLED")
	}
	return nil
}

// redisPingerAdapter wraps *redis.Client to satisfy health.RedisPinger.
type redisPingerAdapter struct {
	rdb *redis.Client
}

func (r redisPingerAdapter) Ping(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

// setupLogging configures the default slog logger from a level string and format.
// When format is "text", a colorized tint handler is used (useful for local dev).
// Otherwise the default JSON handler is used.
func setupLogging(level, format string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	if format == "text" {
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.Kitchen,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}

	slog.SetDefault(slog.New(handler))
}

// logAuditDMLGuardStartup runs once at boot to surface the migration
// 000187 enforcement posture. On self-hosted installs that never
// provisioned the strait_app role the migration silently no-ops —
// without this check that condition is invisible in logs until the
// next chain verification. The guard probe reports the same state on
// every health scrape; this function is the boot-time analogue so the
// first log line already calls out the gap.
func logAuditDMLGuardStartup(ctx context.Context, checker health.AuditDMLPrivilegeChecker, metrics *telemetry.Metrics) {
	if checker == nil {
		return
	}
	recordStatus := func(status string) {
		if metrics == nil || metrics.AuditDMLRestrictionStatus == nil {
			return
		}
		metrics.AuditDMLRestrictionStatus.Add(ctx, 1,
			otelmetric.WithAttributes(otelattr.String("status", status)))
	}
	restricted, err := checker.AuditEventsUpdateRestricted(ctx)
	if err != nil {
		slog.Warn("audit DML restriction probe failed at startup",
			"error", err,
			"hint", "self-hosted installs must run against the strait_app role for migration 000187 to enforce UPDATE restrictions; see SELFHOST.md")
		recordStatus("degraded")
		return
	}
	if !restricted {
		slog.Warn("audit_events UPDATE is not restricted for current role; migration 000187 DML guardrail is a no-op",
			"status", "degraded",
			"hint", "connect the app as the strait_app role (or an equivalent least-privilege role) so the REVOKE UPDATE / GRANT UPDATE (signature) migration takes effect")
		recordStatus("degraded")
		return
	}
	slog.Info("audit_events DML restriction enforced at database role level", "status", "enforced")
	recordStatus("enforced")
}
