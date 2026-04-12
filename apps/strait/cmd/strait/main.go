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
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/webhook"
	"strait/internal/workflow"

	"github.com/getsentry/sentry-go"
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
	case "dispatcher":
		return runDispatcher(ctx, cfg)
	default:
		return fmt.Errorf("invalid mode %q: must be api, worker, all, or dispatcher", cfg.Mode)
	}

	setupLogging(cfg.LogLevel, cfg.LogFormat)

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.SentryDSN,
			Environment:      cfg.SentryEnvironment,
			Release:          version,
			AttachStacktrace: true,
			SampleRate:       1.0,
			TracesSampleRate: 0.1,
			IgnoreErrors: []string{
				"context canceled",
				"context deadline exceeded",
				"connection refused",
				"connection reset by peer",
				"broken pipe",
				"use of closed network connection",
			},
			BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
				if event.Request != nil {
					event.Request.Headers = nil
					event.Request.Cookies = ""
					event.Request.Data = ""
					if event.Request.QueryString != "" {
						event.Request.QueryString = telemetry.SanitizeQueryString(event.Request.QueryString)
					}
				}
				for i := range event.Exception {
					event.Exception[i].Value = telemetry.ScrubSecrets(event.Exception[i].Value)
				}
				event.Message = telemetry.ScrubSecrets(event.Message)
				for i := range event.Breadcrumbs {
					if event.Breadcrumbs[i].Data != nil {
						for _, key := range []string{
							"request_body", "response_body", "headers",
							"authorization", "token", "secret",
						} {
							delete(event.Breadcrumbs[i].Data, key)
						}
					}
				}
				for k, v := range event.Extra {
					if s, ok := v.(string); ok {
						event.Extra[k] = telemetry.SanitizeValue(k, s)
					}
				}
				return event
			},
		}); err != nil {
			return fmt.Errorf("init sentry: %w", err)
		}
		defer sentry.Flush(2 * time.Second)

		// Re-wrap the slog handler to pipe Error-level logs to Sentry.
		currentHandler := slog.Default().Handler()
		sentryLogger := slog.New(telemetry.NewSentryHandler(currentHandler))
		slog.SetDefault(sentryLogger)

		slog.Info("sentry initialized", "environment", cfg.SentryEnvironment)
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
	queries.SetSecretEncryptionKey(cfg.SecretEncryptionKey)
	if cfg.InternalSecret != "" {
		auditKey, auditKeyErr := store.DeriveAuditSigningKey(cfg.InternalSecret)
		if auditKeyErr != nil {
			return fmt.Errorf("derive audit signing key: %w", auditKeyErr)
		}
		queries.SetAuditSigningKey(auditKey)
	}
	q := queue.NewPostgresQueue(dbPool, queue.WithPriorityAging(true))

	pub, rdb, err := connectRedis(ctx, cfg)
	if err != nil {
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
		webhook.WithHTTPTransport(cfg.WebhookTimeout, cfg.WebhookIdleConnTimeout, cfg.WebhookMaxIdleConns, cfg.WebhookMaxIdleConnsPerHost),
		webhook.WithBatchByURL(cfg.WebhookBatchEnabled),
		webhook.WithMaxBatchSize(cfg.WebhookMaxBatchSize),
	)
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

	var apiEncryptor api.Encryptor
	if cfg.EncryptionKey != "" {
		enc, encErr := crypto.NewEncryptor(cfg.EncryptionKey)
		if encErr != nil {
			slog.Warn("failed to create encryptor for API; event source signature encryption disabled", "error", encErr)
		} else {
			apiEncryptor = enc
		}
	}

	// Create a shared billing enforcer (used by both API webhook handler and worker executor).
	// Only created when billing enforcement is enabled or Stripe webhook secret is set.
	var billingEnforcer *billing.Enforcer
	if rdb != nil && (cfg.BillingEnforcementEnabled || cfg.StripeWebhookSecret != "") {
		billingStore := billing.NewPgStore(dbPool)
		var enforcerOpts []billing.EnforcerOption
		billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default())
		if billingEmailSender != nil {
			enforcerOpts = append(enforcerOpts, billing.WithEnforcerBillingEmails(billingEmailSender))
		}
		billingEnforcer = billing.NewEnforcer(billingStore, rdb, slog.Default(), enforcerOpts...)
		billingEnforcer.StartCleanup(ctx)
	}

	if (cfg.BillingEnforcementEnabled || cfg.StripeWebhookSecret != "") && cfg.StripeWebhookSecret == "" {
		slog.Warn("STRIPE_WEBHOOK_SECRET is empty -- Stripe webhook signature verification is DISABLED")
	}

	cdcWebhookReceiver := startCDCConsumer(g, cfg, pub, queries, chExporter)
	startWebhookWorker(g, cfg, eventNotifier)
	startNotificationWorker(g, cfg, queries, metrics)
	startLogDrainWorker(g, cfg, queries, metrics)
	startMaintenanceWorker(g, queries)
	var chAnalytics api.AnalyticsStore
	if chClient != nil {
		chAnalytics = clickhouse.NewAnalyticsStore(chClient, clickhouse.NewPgHealthAdapter(dbPool))
	}
	startAPIServer(g, cfg, queries, dbPool, dbPool, q, pub, metricsHandler, metrics, stepCallback, workflowEngine, healthReg, rdb, apiEncryptor, billingEnforcer, chAnalytics, chExporter, cdcWebhookReceiver)
	startWorker(g, cfg, queries, dbPool, dbPool, q, pub, metrics, stepCallback, workflowEngine, healthReg, billingEnforcer, chExporter)

	if err := g.Wait(); err != nil {
		return fmt.Errorf("services: %w", err)
	}

	slog.Info("strait stopped")
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
