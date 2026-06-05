package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"strait/internal/api"
	grpcserver "strait/internal/api/grpc"
	"strait/internal/billing"
	straitcache "strait/internal/cache"
	"strait/internal/cdc"
	"strait/internal/clickhouse"
	"strait/internal/config"
	"strait/internal/debug"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/httputil"
	"strait/internal/logdrain"
	"strait/internal/notification"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/ratelimit"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/webhook"
	"strait/internal/worker"
	"strait/internal/workflow"
	"strait/migrations"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/exaring/otelpgx"
	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/resend/resend-go/v2"
	"github.com/sourcegraph/conc/pool"
)

func shutdownReason(err error) string {
	if err == nil {
		return "graceful"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "forced"
}

func logWorkerShutdownStart(logger *slog.Logger, startedAt time.Time, inFlightRuns int64, drainTimeout time.Duration) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("worker shutdown initiated",
		"shutdown_started_at", startedAt.UTC().Format(time.RFC3339Nano),
		"in_flight_runs", inFlightRuns,
		"drain_timeout", drainTimeout.String(),
	)
}

func logWorkerShutdownComplete(logger *slog.Logger, metrics *telemetry.Metrics, completedAt time.Time, runsDrained int64, reason string, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	if reason == "" {
		reason = shutdownReason(err)
	}

	if err != nil {
		logger.Warn("worker shutdown completed with warning",
			"shutdown_completed_at", completedAt.UTC().Format(time.RFC3339Nano),
			"runs_drained", runsDrained,
			"reason", reason,
			"error", err,
		)
	} else {
		logger.Info("worker shutdown completed",
			"shutdown_completed_at", completedAt.UTC().Format(time.RFC3339Nano),
			"runs_drained", runsDrained,
			"reason", reason,
		)
	}

	if metrics != nil {
		metrics.ShutdownTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
}

func logProfilingStartup(logger *slog.Logger, cfg *config.Config) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg == nil || !cfg.ProfilingEnabled {
		logger.Info("pprof profiling disabled")
		return
	}

	args := []any{
		"profiling_secret_configured", cfg.ProfilingSecret != "",
		"cidr_allowlist_configured", len(cfg.ProfilingAllowedCIDRs) > 0,
		"api_listener", profilingAPIListenerEnabled(cfg),
		"management_listener", profilingManagementListenerEnabled(cfg),
		"mutex_fraction", cfg.ProfilingMutexFraction,
		"block_rate", cfg.ProfilingBlockRate,
		"cpu_profile_max_seconds", debug.MaxPprofProfileSeconds,
	}
	if profilingManagementListenerEnabled(cfg) {
		args = append(args, "management_bind_addr", profilingManagementAddr(cfg))
	}
	logger.Warn("pprof profiling enabled", args...)
}

// connectDatabase creates and verifies a Postgres connection pool.
// It retries with exponential backoff up to 5 times on transient failures.
func connectDatabase(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if cfg.DBPgBouncerMode && !cfg.DBPgBouncerPrepared {
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	if cfg.DBHealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = cfg.DBHealthCheckPeriod
	}
	if cfg.DBTraceStatements {
		poolConfig.ConnConfig.Tracer = multitracer.New(
			otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName()),
			telemetry.SentryPGXTracer{},
		)
	}

	// These MVCC horizon guardrails and timeouts are applied to every connection in the pool via pgx's
	// StartupMessage. They prevent stray long transactions from pinning pg_xmin
	// and blocking autovacuum on hot queue tables.
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ApplyDBRuntimeParams(poolConfig.ConnConfig.RuntimeParams, cfg)
	// transaction_timeout is Postgres 17+; set via AfterConnect and ignore errors
	// so the pool still boots on older servers.
	if cfg.DBTransactionTimeout > 0 {
		prevAfterConnect := poolConfig.AfterConnect
		timeoutMs := cfg.DBTransactionTimeout.Milliseconds()
		poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			if prevAfterConnect != nil {
				if err := prevAfterConnect(ctx, conn); err != nil {
					return err
				}
			}
			if _, err := conn.Exec(ctx, fmt.Sprintf("SET transaction_timeout = %d", timeoutMs)); err != nil {
				slog.Debug("transaction_timeout unsupported on this postgres version", "error", err)
			}
			return nil
		}
	}

	const maxRetries = 5
	var pool *pgxpool.Pool
	for attempt := range maxRetries {
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			slog.Warn("failed to connect to postgres, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err,
			)
			if err := retrySleep(ctx, attempt); err != nil {
				return nil, fmt.Errorf("connect to postgres cancelled: %w", err)
			}
			continue
		}

		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			slog.Warn("failed to ping postgres, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err,
			)
			if err := retrySleep(ctx, attempt); err != nil {
				return nil, fmt.Errorf("ping postgres cancelled: %w", err)
			}
			continue
		}

		slog.Info("connected to postgres",
			"max_conns", cfg.DBMaxConns,
			"min_conns", cfg.DBMinConns,
			"max_conn_lifetime", cfg.DBMaxConnLifetime,
			"max_conn_idle_time", cfg.DBMaxConnIdleTime,
			"health_check_period", cfg.DBHealthCheckPeriod,
		)
		return pool, nil
	}

	return nil, fmt.Errorf("connect to postgres: failed after %d retries: %w", maxRetries, err)
}

// connectRedis creates and verifies the required Redis client for pub/sub.
// It retries with exponential backoff up to 5 times on transient failures.
func connectRedis(ctx context.Context, cfg *config.Config) (pubsub.Publisher, *redis.Client, error) {
	rdb, err := pubsub.NewRedisClient(cfg.RedisURL, cfg.RedisSentinelMaster, cfg.RedisSentinelAddrs, pubsub.RedisPoolOptions{
		PoolSize:        cfg.RedisPoolSize,
		MinIdleConns:    cfg.RedisMinIdleConns,
		ReadTimeout:     cfg.RedisReadTimeout,
		WriteTimeout:    cfg.RedisWriteTimeout,
		ConnMaxLifetime: cfg.RedisConnMaxLifetime,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create redis client: %w", err)
	}

	const maxRetries = 5
	for attempt := range maxRetries {
		if err := rdb.Ping(ctx).Err(); err != nil {
			slog.Warn("failed to ping redis, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err,
			)
			if err := retrySleep(ctx, attempt); err != nil {
				return nil, nil, fmt.Errorf("ping redis cancelled: %w", err)
			}
			continue
		}

		if cfg.RedisSentinelMaster != "" {
			slog.Info("connected to redis via sentinel", "master", cfg.RedisSentinelMaster)
		} else {
			slog.Info("connected to redis")
		}
		rdb.AddHook(telemetry.RedisBreadcrumbHook{})
		rdb.AddHook(telemetry.NewRedisMetricsHook("default", rdb))
		pub := pubsub.NewRedisPublisher(rdb)
		return pub, rdb, nil
	}

	return nil, nil, fmt.Errorf("ping redis: failed after %d retries", maxRetries)
}

func waitForSequin(ctx context.Context, client *cdc.Client) error {
	const maxRetries = 5
	var lastErr error
	for attempt := range maxRetries {
		if err := client.Health(ctx); err != nil {
			lastErr = err
			slog.Warn("failed to reach sequin, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err,
			)
			if err := retrySleep(ctx, attempt); err != nil {
				return fmt.Errorf("sequin health check cancelled: %w", err)
			}
			continue
		}
		slog.Info("connected to sequin")
		return nil
	}
	return fmt.Errorf("connect to sequin: failed after %d retries: %w", maxRetries, lastErr)
}

// retrySleep sleeps with exponential backoff: 1s, 2s, 4s, 8s, 16s.
// Returns an error if the context is cancelled during the sleep.
func retrySleep(ctx context.Context, attempt int) error {
	delay := min(time.Second<<uint(max(attempt, 0)), 16*time.Second)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

type cdcHandlerRegistrar interface {
	RegisterHandler(cdc.Handler)
	RegisterAdditionalHandler(cdc.Handler)
}

func registerCDCDeliveryHandlers(registrar cdcHandlerRegistrar, pub pubsub.Publisher, queries *store.Queries, chExporter *clickhouse.Exporter, cacheHandlers cdc.CacheReadModelHandlers, cacheBus *straitcache.Bus, sharedDedupe *cdc.SharedDedupeStore) {
	for _, handler := range cdc.NewRuntimeFanoutHandlers(pub, slog.Default()) {
		registrar.RegisterHandler(handler)
	}

	// CDC-driven observers: execution-critical side effects are written through
	// transactional stores, not CDC redelivery.
	registrar.RegisterAdditionalHandler(cdc.NewNotificationTriggerHandler(queries, slog.Default()))
	registrar.RegisterAdditionalHandler(cdc.NewSLOHandler(queries, slog.Default()).WithSharedDedupe(sharedDedupe))
	if chExporter != nil {
		registrar.RegisterAdditionalHandler(cdc.NewAnalyticsHandler(chExporter, slog.Default()).WithSharedDedupe(sharedDedupe))
	}
	if cacheHandlers.JobRuns != nil {
		registrar.RegisterAdditionalHandler(cacheHandlers.JobRuns)
		registrar.RegisterAdditionalHandler(cacheHandlers.WorkflowRuns)
		registrar.RegisterAdditionalHandler(cacheHandlers.WorkflowStepRuns)
	}
	for _, h := range cdc.NewCacheInvalidationHandlers(cacheBus, slog.Default()) {
		registrar.RegisterAdditionalHandler(h)
	}
}

// startCDCConsumer registers and starts the required Sequin CDC consumer.
func startCDCConsumer(ctx context.Context, g *pool.ContextPool, cfg *config.Config, pub pubsub.Publisher, queries *store.Queries, chExporter *clickhouse.Exporter, rdb *redis.Client, cacheBus *straitcache.Bus) (*cdc.WebhookReceiver, error) {
	cdcClient := cdc.NewClient(
		cfg.SequinBaseURL,
		cfg.SequinConsumerName,
		cfg.SequinAPIToken,
		cdc.WithDatabaseName(cfg.SequinDatabaseName),
	)
	if err := waitForSequin(ctx, cdcClient); err != nil {
		return nil, err
	}

	// Auto-provision the Sequin consumer if it does not exist.
	if err := cdcClient.EnsureConsumer(ctx, cdc.RequiredConsumerTables()); err != nil {
		return nil, fmt.Errorf("ensure sequin consumer %q: %w", cfg.SequinConsumerName, err)
	}

	cdcConsumer := cdc.NewConsumer(cdcClient, cdc.ConsumerConfig{
		BaseURL:      cfg.SequinBaseURL,
		ConsumerName: cfg.SequinConsumerName,
		Credential:   cfg.SequinAPIToken,
		BatchSize:    cfg.CDCBatchSize,
		WaitTimeMs:   cfg.CDCWaitTimeMs,
	}, slog.Default())
	sharedDedupe := cdc.NewSharedDedupeStore(rdb, cfg.SharedDedupeTTL)
	cacheHandlers := cdc.NewCacheReadModelHandlers(rdb, cfg.StatusReadModelTTL, slog.Default())
	registerCDCDeliveryHandlers(cdcConsumer, pub, queries, chExporter, cacheHandlers, cacheBus, sharedDedupe)

	g.Go(func(ctx context.Context) error {
		cdcConsumer.Run(ctx)
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("draining cdc consumer")
		drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.WorkerDrainTimeout)
		defer drainCancel()
		if err := cdcConsumer.Shutdown(drainCtx); err != nil {
			return err
		}
		slog.Info("cdc consumer drained")
		return nil
	})

	// Create webhook receiver for push-based CDC delivery.
	receiverOpts := []cdc.WebhookReceiverOption{}
	if cfg.SequinWebhookSecret != "" {
		receiverOpts = append(receiverOpts, cdc.WithWebhookSecret(cfg.SequinWebhookSecret))
	} else {
		slog.Warn("cdc webhook signature verification disabled: SEQUIN_WEBHOOK_SECRET is not set")
	}
	receiverOpts = append(receiverOpts, cdc.WithWebhookSharedDedupe(sharedDedupe))
	webhookReceiver := cdc.NewWebhookReceiver(pub, slog.Default(), receiverOpts...)
	registerCDCDeliveryHandlers(webhookReceiver, pub, queries, chExporter, cacheHandlers, cacheBus, sharedDedupe)

	slog.Info("cdc consumer enabled",
		"base_url", httputil.RedactURLForLog(cfg.SequinBaseURL),
		"consumer", cfg.SequinConsumerName,
	)

	return webhookReceiver, nil
}

func startWebhookWorker(g *pool.ContextPool, cfg *config.Config, eventNotifier *webhook.DeliveryWorker) {
	g.Go(func(ctx context.Context) error {
		return eventNotifier.RunWorker(ctx, 5*time.Second)
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("draining webhook delivery worker")
		drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.WorkerDrainTimeout)
		defer drainCancel()
		if err := eventNotifier.Shutdown(drainCtx); err != nil {
			return err
		}
		slog.Info("webhook delivery worker drained")
		return nil
	})
}

func notificationWorkerEnabled(mode string) bool {
	return mode == "worker" || mode == "all"
}

func startNotificationWorker(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, metrics *telemetry.Metrics) {
	if cfg == nil || !notificationWorkerEnabled(cfg.Mode) {
		return
	}

	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: httputil.NewExternalTransport(cfg.AllowPrivateEndpoints),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	notifWorker := notification.NewWorkerWithEmail(
		queries,
		httpClient,
		cfg.ResendAPIKey,
		cfg.ResendFromEmail,
		notification.WithWebhookAllowPrivateEndpoints(cfg.AllowPrivateEndpoints),
	)
	if metrics != nil {
		notifWorker.WithDeliveriesCounter(metrics.NotificationDeliveriesTotal)
	}

	g.Go(func(ctx context.Context) error {
		notifWorker.Start(ctx)
		<-ctx.Done()
		slog.Info("stopping notification worker")
		notifWorker.Stop()
		slog.Info("notification worker stopped")
		return nil
	})
}

func startLogDrainWorker(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, metrics *telemetry.Metrics) {
	if cfg == nil || !notificationWorkerEnabled(cfg.Mode) {
		return
	}

	svc := logdrain.NewService()
	w := logdrain.NewWorker(queries, svc, 30*time.Second)
	if metrics != nil {
		w.WithEventsCounter(metrics.LogDrainEventsTotal)
	}

	g.Go(func(ctx context.Context) error {
		w.Run(ctx)
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("stopping log drain worker")
		w.Stop()
		slog.Info("log drain worker stopped")
		return nil
	})
}

type apiServerDeps struct {
	group              *pool.ContextPool
	config             *config.Config
	queries            *store.Queries
	txPool             store.TxBeginner
	dbPool             *pgxpool.Pool
	queue              queue.Queue
	publisher          pubsub.Publisher
	cacheRegistry      *straitcache.Registry
	cacheBus           *straitcache.Bus
	metricsHandler     http.Handler
	metrics            *telemetry.Metrics
	stepCallback       *workflow.StepCallback
	workflowEngine     *workflow.WorkflowEngine
	healthRegistry     *health.Registry
	redisClient        *redis.Client
	encryptor          api.Encryptor
	billingEnforcer    *billing.Enforcer
	analyticsStore     api.AnalyticsStore
	clickHouseExporter *clickhouse.Exporter
	cdcWebhookReceiver *cdc.WebhookReceiver
}

// startAPIServer starts the HTTP API server and its graceful shutdown goroutine.
func startAPIServer(deps apiServerDeps) {
	g := deps.group
	cfg := deps.config
	queries := deps.queries
	billingEnforcer := deps.billingEnforcer

	if cfg.Mode != "api" && cfg.Mode != "all" {
		return
	}

	billingStore := billing.NewPgStore(deps.dbPool)
	stripeWebhook := buildStripeWebhook(g, cfg, queries, billingStore, billingEnforcer)
	usageSvc := buildUsageService(billingStore, billingEnforcer)
	siemDrain := buildAuditSIEMDrain(cfg)

	srv := api.NewServer(api.ServerDeps{
		Config:             cfg,
		Store:              queries,
		AnalyticsStore:     deps.analyticsStore,
		Queue:              deps.queue,
		PubSub:             deps.publisher,
		MetricsHandler:     deps.metricsHandler,
		Metrics:            deps.metrics,
		Pinger:             pingerFromPublisher(deps.publisher),
		HealthRegistry:     deps.healthRegistry,
		WorkflowCallback:   deps.stepCallback,
		WorkflowEngine:     deps.workflowEngine,
		TxPool:             deps.txPool,
		RedisClient:        deps.redisClient,
		CacheBus:           deps.cacheBus,
		CacheRegistry:      deps.cacheRegistry,
		Encryptor:          deps.encryptor,
		StripeWebhook:      stripeWebhook,
		BillingEnforcer:    nilSafeBillingEnforcer(billingEnforcer),
		UsageService:       usageSvc,
		CHExporter:         deps.clickHouseExporter,
		Edition:            domain.ParseEdition(cfg.Edition),
		Version:            version,
		CDCWebhookReceiver: deps.cdcWebhookReceiver,
		SIEMDrain:          siemDrain,
	})
	registerAPIMetrics(deps.metrics, srv, siemDrain)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	g.Go(func(context.Context) error {
		slog.Info("api server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("shutting down api server")
		srv.Close()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
}

func pingerFromPublisher(pub pubsub.Publisher) api.Pinger {
	redisPub, ok := pub.(*pubsub.RedisPublisher)
	if !ok {
		return nil
	}
	return redisPub
}

func buildStripeWebhook(
	g *pool.ContextPool,
	cfg *config.Config,
	queries *store.Queries,
	billingStore *billing.PgStore,
	billingEnforcer *billing.Enforcer,
) http.Handler {
	if cfg.StripeWebhookSecret == "" {
		return nil
	}

	stripeMapping := billing.NewStripeMappingFromOptions(
		billing.WithStarterPrices(cfg.StripeStarterMonthlyPriceID, cfg.StripeStarterYearlyPriceID),
		billing.WithProPrices(cfg.StripeProMonthlyPriceID, cfg.StripeProYearlyPriceID),
		billing.WithScalePrices(cfg.StripeScaleMonthlyPriceID, cfg.StripeScaleYearlyPriceID),
		billing.WithBusinessPrices(cfg.StripeBusinessMonthlyPriceID, cfg.StripeBusinessYearlyPriceID),
		billing.WithEnterpriseStarterPrice(cfg.StripeEnterpriseStarterYearlyPriceID),
		billing.WithEnterpriseGrowthPrice(cfg.StripeEnterpriseGrowthYearlyPriceID),
		billing.WithEnterpriseLargePrice(cfg.StripeEnterpriseLargeYearlyPriceID),
		billing.WithStarterFlatPrice(cfg.StripeStarterPriceID),
		billing.WithProFlatPrice(cfg.StripeProPriceID),
		billing.WithScaleFlatPrice(cfg.StripeScalePriceID),
		billing.WithEnterpriseFlatPrice(cfg.StripeEnterprisePriceID),
	)
	wh := billing.NewWebhookHandler(
		billingStore,
		stripeMapping,
		cfg.StripeWebhookSecret,
		slog.Default(),
		billingEnforcer,
		queries,
		stripeWebhookOptions(cfg, billingStore)...,
	)
	g.Go(func(ctx context.Context) error {
		wh.StartReplayCleanup(ctx)
		<-ctx.Done()
		return nil
	})
	slog.Info("stripe webhook handler enabled")
	return wh
}

func stripeWebhookOptions(cfg *config.Config, billingStore *billing.PgStore) []billing.WebhookOption {
	opts := []billing.WebhookOption{}
	if posthogClient := billing.NewPostHogClient(cfg.PostHogAPIKey, cfg.PostHogHost, slog.Default()); posthogClient != nil {
		opts = append(opts, billing.WithPostHog(posthogClient))
	}
	if cfg.ResendAPIKey != "" {
		resendClient := billing.NewResendWelcomeEmailFunc(cfg.ResendAPIKey, cfg.ResendFromEmail)
		opts = append(opts, billing.WithWelcomeEmail(resendClient))
	}
	if billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default()); billingEmailSender != nil {
		opts = append(opts, billing.WithBillingEmails(billingEmailSender))
	}
	opts = append(opts, billing.WithEdition(cfg.Edition))
	opts = append(opts, billing.WithDunningStore(billingStore))
	return opts
}

func buildUsageService(billingStore *billing.PgStore, billingEnforcer *billing.Enforcer) *billing.UsageService {
	if billingEnforcer == nil {
		return nil
	}
	return billing.NewUsageService(billingStore, billingEnforcer)
}

func buildAuditSIEMDrain(cfg *config.Config) *logdrain.AuditSIEMDrain {
	siemDrain := logdrain.NewAuditSIEMDrain(
		cfg.AuditSIEMEndpoint,
		cfg.AuditSIEMAuthToken,
		cfg.AuditSIEMBatchSize,
		cfg.AuditSIEMFlushInterval,
	)
	if siemDrain != nil {
		slog.Info("audit SIEM drain enabled",
			"endpoint", httputil.RedactURLForLog(cfg.AuditSIEMEndpoint),
			"batch_size", cfg.AuditSIEMBatchSize,
			"flush_interval", cfg.AuditSIEMFlushInterval)
	}
	return siemDrain
}

func registerAPIMetrics(metrics *telemetry.Metrics, srv *api.Server, siemDrain *logdrain.AuditSIEMDrain) {
	if metrics == nil {
		return
	}
	if err := metrics.ObserveAuditDrainer(otel.Meter("strait"), srv); err != nil {
		slog.Warn("failed to register audit drainer metrics callback", "error", err)
	}
	if siemDrain == nil {
		return
	}
	if err := metrics.ObserveSIEMBreakerState(otel.Meter("strait"), siemDrain); err != nil {
		slog.Warn("failed to register SIEM breaker state metrics callback", "error", err)
	}
}

func startProfilingServer(g *pool.ContextPool, cfg *config.Config, rdb *redis.Client, metrics *telemetry.Metrics, version string) {
	if !profilingManagementListenerEnabled(cfg) {
		return
	}

	handler := api.NewProfilingHandler(api.ProfilingHandlerDeps{
		Config:      cfg,
		RedisClient: rdb,
		Metrics:     metrics,
		Edition:     domain.ParseEdition(cfg.Edition),
		Version:     version,
	})
	httpServer := &http.Server{
		Addr:              profilingManagementAddr(cfg),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	g.Go(func(context.Context) error {
		slog.Info("profiling management server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("profiling management server: %w", err)
		}
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("shutting down profiling management server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
}

func profilingManagementListenerEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.ProfilingEnabled && cfg.ProfilingManagementEnabled
}

func profilingAPIListenerEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.ProfilingEnabled {
		return false
	}
	if cfg.ProfilingManagementEnabled && !cfg.ProfilingAPIEnabled {
		return false
	}
	return cfg.ProfilingAPIEnabled || !cfg.ProfilingManagementEnabled
}

func profilingManagementAddr(cfg *config.Config) string {
	return net.JoinHostPort(cfg.ProfilingManagementBindAddr, strconv.Itoa(cfg.ProfilingManagementPort))
}

// startGRPCServer starts the gRPC server for the Worker streaming API.
// It is symmetric to startAPIServer: the server shuts down before the HTTP
// server on SIGTERM so that connected workers can reconnect to other replicas
// before the HTTP surface disappears.
func startGRPCServer(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, pub pubsub.Publisher, rdb *redis.Client, q queue.Queue, billingEnforcer *billing.Enforcer, version string, decryptor grpcserver.SecretDecryptor) (*grpcserver.Server, error) {
	if cfg.Mode != "api" && cfg.Mode != "all" {
		return nil, nil
	}
	if !cfg.GRPCEnabled {
		return nil, nil
	}
	if pub == nil {
		// Refuse to boot rather than serve a worker stream that will panic
		// the first time a worker connects (subscribe / publish on a nil
		// publisher). Operators see a clear cause instead of a recovered
		// internal error after the fact.
		return nil, fmt.Errorf("grpc worker plane is enabled but no pubsub publisher is configured: set REDIS_URL or disable GRPC_ENABLED")
	}
	if err := waitForPubsubReady(context.Background(), pub, cfg.GRPCPubsubStartupTimeout); err != nil {
		slog.Error("service.startup.gate", "component", "pubsub", "result", "timeout", "error", err)
		return nil, err
	}
	slog.Info("service.startup.gate", "component", "pubsub", "result", "ok")

	opts := []grpcserver.ServerOption{
		grpcserver.WithAuthLimiter(ratelimit.NewAuthLimiter(rdb, true)),
		grpcserver.WithAPIKeyCache(rdb, cfg.APIKeyCacheTTL),
		grpcserver.WithVersion(version),
		grpcserver.WithSecretDecryptor(decryptor),
	}
	if billingEnforcer != nil {
		opts = append(opts, grpcserver.WithBillingEnforcer(billingEnforcer))
	}
	if readyRunQueue, ok := q.(grpcserver.ReadyRunEnqueuer); ok {
		opts = append(opts, grpcserver.WithReadyRunEnqueuer(readyRunQueue))
	}
	srv, err := grpcserver.NewServer(cfg, queries, pub, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc server: %w", err)
	}
	g.Go(func(ctx context.Context) error {
		if err := srv.Serve(ctx); err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	})
	return srv, nil
}

type pubsubPinger interface {
	Ping(context.Context) error
}

func waitForPubsubReady(ctx context.Context, pub pubsub.Publisher, budget time.Duration) error {
	pinger, ok := pub.(pubsubPinger)
	if !ok {
		return fmt.Errorf("grpc worker plane pubsub publisher does not support readiness ping")
	}
	if budget <= 0 {
		budget = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	backoffs := []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, time.Second}
	attempt := 0
	for {
		if err := pinger.Ping(ctx); err == nil {
			return nil
		}
		delay := backoffs[min(attempt, len(backoffs)-1)]
		attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("grpc worker plane pubsub readiness timeout after %s: %w", budget, ctx.Err())
		case <-timer.C:
		}
	}
}

type workerRuntimeDeps struct {
	group              *pool.ContextPool
	config             *config.Config
	queries            *store.Queries
	txPool             store.TxBeginner
	dbPool             *pgxpool.Pool
	queue              queue.Queue
	backpressure       *queue.Backpressure
	publisher          pubsub.Publisher
	cacheRegistry      *straitcache.Registry
	cacheBus           *straitcache.Bus
	redisClient        *redis.Client
	metrics            *telemetry.Metrics
	stepCallback       *workflow.StepCallback
	workflowEngine     *workflow.WorkflowEngine
	healthRegistry     *health.Registry
	billingEnforcer    *billing.Enforcer
	billingDispatcher  *webhook.BillingDispatcher
	clickHouseExporter *clickhouse.Exporter
	workerPlane        *grpcserver.Server
	encryptor          api.Encryptor
}

func startCacheBus(g *pool.ContextPool, pub pubsub.Publisher) (*straitcache.Registry, *straitcache.Bus) {
	registry := straitcache.NewRegistry(straitcache.RegistryConfig{})
	bus := straitcache.NewBus(pub, straitcache.BusConfig{Origin: registry.Origin()})
	g.Go(func(ctx context.Context) error {
		return bus.Run(ctx, registry)
	})
	slog.Info("cachebus subscriber enabled",
		"channel", bus.Channel(),
		"origin", registry.Origin(),
	)
	return registry, bus
}

// startWorker starts the job executor, worker pool, and scheduler goroutines.
func startWorker(deps workerRuntimeDeps) {
	g := deps.group
	cfg := deps.config
	queries := deps.queries
	pub := deps.publisher
	metrics := deps.metrics
	healthReg := deps.healthRegistry
	billingEnforcer := deps.billingEnforcer
	chExporter := deps.clickHouseExporter
	workerPlane := deps.workerPlane

	if cfg.Mode != "worker" && cfg.Mode != "all" {
		return
	}

	var poolOpts []worker.PoolOption
	if cfg.WorkerQueueSize > 0 {
		poolOpts = append(poolOpts, worker.WithQueueSize(cfg.WorkerQueueSize))
	}

	adaptive := worker.NewAdaptiveConcurrency(cfg.AdaptiveConcurrencyMin, cfg.AdaptiveConcurrencyMax, cfg.WorkerConcurrency)
	poolConcurrency := max(adaptive.CurrentLimit(), cfg.AdaptiveConcurrencyMax)
	slog.Info(
		"adaptive worker concurrency enabled",
		"min", cfg.AdaptiveConcurrencyMin,
		"max", cfg.AdaptiveConcurrencyMax,
		"initial", adaptive.CurrentLimit(),
	)

	p := worker.NewPool(poolConcurrency, poolOpts...)
	notifier := queue.NewQueueNotifier(cfg.DatabaseURL, slog.Default())
	wake := notifier.Wake()
	g.Go(func(ctx context.Context) error {
		notifier.Run(ctx)
		return nil
	})
	slog.Info("worker queue listen/notify enabled", "channel", queue.QueueWakeChannel)

	execCfg := buildExecutorConfig(deps, p, wake, notifier, adaptive)
	applyWorkerBillingConfig(&execCfg, cfg, billingEnforcer, metrics)

	exec := worker.NewExecutor(execCfg)
	if workerPlane != nil {
		workerPlane.SetRunResultFinalizer(exec)
	}

	registerWorkerSubscribers(exec, pub, metrics, chExporter, queries)
	registerWorkerHealthChecks(healthReg, p, queries, cfg.WorkerConcurrency)
	registerWorkerMetrics(g, metrics, p, queries, chExporter)

	g.Go(func(ctx context.Context) error {
		if adaptive != nil {
			adaptive.Run(ctx, 3*time.Second, func(probeCtx context.Context) (int, float64, error) {
				stats, err := queries.QueueStats(probeCtx)
				if err != nil {
					return 0, 0, err
				}
				current := adaptive.CurrentLimit()
				current = max(current, 1)
				utilization := float64(p.ActiveCount()) / float64(current)
				return stats.Queued, utilization, nil
			}, slog.Default())
		}
		return nil
	})

	g.Go(func(ctx context.Context) error {
		exec.Run(ctx)
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("draining executor")
		drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.WorkerDrainTimeout)
		defer drainCancel()
		if err := exec.Shutdown(drainCtx); err != nil {
			return err
		}
		slog.Info("executor drained")
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		shutdownStartedAt := time.Now()
		inFlightRuns := p.RunningWorkers()
		runCompletedBefore := p.CompletedTasks()
		logWorkerShutdownStart(slog.Default(), shutdownStartedAt, inFlightRuns, cfg.WorkerDrainTimeout)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.WorkerDrainTimeout)
		defer shutdownCancel()
		err := p.Shutdown(shutdownCtx)
		runCompletedAfter := p.CompletedTasks()
		runsDrainedU64 := uint64(0)
		if runCompletedAfter >= runCompletedBefore {
			runsDrainedU64 = runCompletedAfter - runCompletedBefore
		}
		runsDrained := int64(min(runsDrainedU64, uint64(math.MaxInt64)))
		logWorkerShutdownComplete(slog.Default(), metrics, time.Now(), runsDrained, shutdownReason(err), err)
		return nil
	})

	g.Go(func(ctx context.Context) error {
		return runWorkerScheduler(ctx, deps)
	})
}

func runWorkerScheduler(ctx context.Context, deps workerRuntimeDeps) error {
	cfg := deps.config
	queries := deps.queries

	schedOpts := baseSchedulerOptions(deps)
	schedOpts = appendPartitionReclaimer(schedOpts, cfg, queries)
	schedOpts = appendBillingSchedulerOptions(schedOpts, deps)
	schedOpts = appendBackpressureSampler(schedOpts, deps)
	if deps.encryptor != nil {
		schedOpts = append(schedOpts, scheduler.WithRotationSecretDecryptor(deps.encryptor))
	}
	schedOpts = append(schedOpts, scheduler.WithSentryRuntime(cfg.Mode, cfg.DefaultRegion, version))

	sched := scheduler.New(ctx, cfg, queries, deps.queue, deps.stepCallback, deps.workflowEngine, schedOpts...)
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	<-ctx.Done()
	slog.Info("shutting down scheduler")
	sched.Stop()
	return nil
}

func baseSchedulerOptions(deps workerRuntimeDeps) []scheduler.SchedulerOption {
	queries := deps.queries
	dbPool := deps.dbPool
	opts := []scheduler.SchedulerOption{
		scheduler.WithSchedulerMetrics(deps.metrics),
		scheduler.WithChExporter(deps.clickHouseExporter),
		scheduler.WithReaperAdvisoryLocker(queries),
		scheduler.WithIndexMaintainerAdvisoryLocker(queries),
		scheduler.WithPriorityPromoter(
			scheduler.NewPriorityPromoter(dbPool, scheduler.PriorityPromoterConfig{
				Interval:     60 * time.Second,
				AgeThreshold: 5 * time.Minute,
				MaxPriority:  1000,
				BatchLimit:   500,
				Logger:       slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithCounterReconciler(
			scheduler.NewCounterReconciler(dbPool, scheduler.CounterReconcilerConfig{
				Interval: time.Hour,
				Logger:   slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithPartitionEnsurer(
			scheduler.NewPartitionEnsurer(queries, scheduler.PartitionEnsurerConfig{
				Interval:    24 * time.Hour,
				MonthsAhead: 2,
				Logger:      slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithPartitionTuner(
			scheduler.NewPartitionTuner(queries, scheduler.PartitionTunerConfig{
				Interval: 7 * 24 * time.Hour,
				Logger:   slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithDLQAgeOut(
			scheduler.NewDLQAgeOut(queries, scheduler.DLQAgeOutConfig{
				Interval:   24 * time.Hour,
				Retention:  30 * 24 * time.Hour,
				BatchLimit: 1000,
				Logger:     slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithOutboxFlusher(
			scheduler.NewOutboxFlusher(dbPool, deps.queue, scheduler.OutboxFlusherConfig{
				Interval:  time.Second,
				BatchSize: 500,
				Logger:    slog.Default(),
			}),
		),
		scheduler.WithOutboxArchiver(
			scheduler.NewOutboxArchiver(queries, scheduler.OutboxArchiverConfig{
				Interval:  time.Second,
				BatchSize: 500,
				Logger:    slog.Default(),
			}),
		),
		scheduler.WithPlanDriftMonitor(
			scheduler.NewPlanDriftMonitor(queries, scheduler.PlanDriftMonitorConfig{
				Queries:  scheduler.DefaultWatchedQueries(),
				Interval: 24 * time.Hour,
				Logger:   slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithIdempotencyGC(
			scheduler.NewIdempotencyGC(queries, scheduler.IdempotencyGCConfig{
				Interval:   time.Hour,
				BatchLimit: 10000,
				Logger:     slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithHeartbeatGC(
			scheduler.NewHeartbeatGC(queries, scheduler.HeartbeatGCConfig{
				Interval:   time.Hour,
				BatchLimit: 10000,
				Logger:     slog.Default(),
			}).WithAdvisoryLocker(queries),
		),
		scheduler.WithSLOEvaluator(
			scheduler.NewSLOEvaluator(queries, slog.Default(),
				scheduler.WithSLOWebhookNotifier(scheduler.NewSLOWebhookAdapter(queries)),
				scheduler.WithSLOEvaluatorAdvisoryLocker(queries),
			),
		),
	}
	if repairer, ok := deps.queue.(scheduler.ReadyRunRepairer); ok {
		opts = append(opts, scheduler.WithReadyRunReconciler(
			scheduler.NewReadyRunReconciler(repairer, 5*time.Minute, 1000),
		))
	}
	return opts
}

func appendPartitionReclaimer(
	opts []scheduler.SchedulerOption,
	cfg *config.Config,
	queries *store.Queries,
) []scheduler.SchedulerOption {
	if !cfg.TerminalArchiveEnabled || !cfg.PartitionReclaimEnabled {
		return opts
	}
	reclaimer := scheduler.NewPartitionReclaimer(queries, scheduler.PartitionReclaimerConfig{
		Interval:     cfg.PartitionReclaimInterval,
		SafetyMonths: cfg.PartitionReclaimSafety,
		Logger:       slog.Default(),
	}).WithAdvisoryLocker(queries)
	opts = append(opts, scheduler.WithPartitionReclaimer(reclaimer))
	slog.Info("partition reclaimer enabled",
		"interval", cfg.PartitionReclaimInterval,
		"safety_months", cfg.PartitionReclaimSafety,
	)
	return opts
}

func appendBillingSchedulerOptions(opts []scheduler.SchedulerOption, deps workerRuntimeDeps) []scheduler.SchedulerOption {
	cfg := deps.config
	queries := deps.queries
	billingCostAccountingEnabled := cfg.BillingEnforcementEnabled || cfg.StripeWebhookSecret != ""
	if !billingCostAccountingEnabled {
		return opts
	}

	schedulerBillingStore := billing.NewPgStore(deps.dbPool)
	if cfg.BillingEnforcementEnabled && deps.billingEnforcer != nil {
		opts = appendBillingEnforcementSchedulerOptions(opts, deps, schedulerBillingStore)
	}
	opts = append(opts,
		scheduler.WithUsageFlusher(
			scheduler.NewUsageFlusher(schedulerBillingStore, time.Hour).WithAdvisoryLocker(queries),
		),
	)
	slog.Info("billing usage flusher enabled")
	return opts
}

func appendBillingEnforcementSchedulerOptions(
	opts []scheduler.SchedulerOption,
	deps workerRuntimeDeps,
	schedulerBillingStore *billing.PgStore,
) []scheduler.SchedulerOption {
	cfg := deps.config
	queries := deps.queries
	billingEnforcer := deps.billingEnforcer
	billingDispatcher := deps.billingDispatcher

	reconciler := scheduler.NewConcurrentReconciler(billingEnforcer, queries, 5*time.Minute)
	opts = append(opts, scheduler.WithConcurrentReconciler(reconciler))
	slog.Info("concurrent run reconciler enabled")

	budgetStore := newBudgetMonitorStore(schedulerBillingStore, queries)
	opts = append(opts, scheduler.WithBudgetMonitoringStores(budgetStore, budgetStore, billingEnforcer))
	slog.Info("billing budget monitors enabled")

	downgradeApplier := scheduler.NewDowngradeApplier(schedulerBillingStore, billingEnforcer, 5*time.Minute)
	if billingDispatcher != nil {
		downgradeApplier.WithBillingDispatcher(billingDispatcher)
	}
	opts = append(opts, scheduler.WithDowngradeApplier(downgradeApplier))
	slog.Info("downgrade applier enabled")

	gracePeriodEnforcer := scheduler.NewGracePeriodEnforcer(schedulerBillingStore, billingEnforcer, time.Hour).
		WithAdvisoryLocker(queries)
	opts = append(opts, scheduler.WithGracePeriodEnforcer(gracePeriodEnforcer))
	slog.Info("grace period enforcer enabled")

	opts = append(opts, scheduler.WithWebhookMessageCleanup(
		scheduler.NewWebhookMessageCleanup(schedulerBillingStore, slog.Default()),
	))

	billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default())
	contractExpiryChecker := scheduler.NewContractExpiryChecker(schedulerBillingStore, billingEmailSender, 24*time.Hour).
		WithOrgCacheInvalidator(billingEnforcer)
	opts = append(opts, scheduler.WithContractExpiryChecker(contractExpiryChecker))
	slog.Info("contract expiry checker enabled")

	opts = appendUsageReportEmailer(opts, cfg, schedulerBillingStore)
	opts = append(opts, scheduler.WithOrgRetentionResolver(billing.NewPlanRetentionResolver(schedulerBillingStore)))
	slog.Info("per-org plan retention enabled")

	quotaResumeEnforcer := scheduler.NewQuotaResumeEnforcer(schedulerBillingStore, billingEnforcer, time.Hour).
		WithAdvisoryLocker(queries)
	opts = append(opts, scheduler.WithQuotaResumeEnforcer(quotaResumeEnforcer))
	slog.Info("quota resume enforcer enabled")

	anomalyMonitor := scheduler.NewAnomalyMonitor(
		&anomalyMonitorStore{PgStore: schedulerBillingStore, queries: queries},
		15*time.Minute,
	).WithAdvisoryLocker(queries)
	opts = append(opts, scheduler.WithAnomalyMonitor(anomalyMonitor))
	slog.Info("anomaly monitor enabled")

	opts = appendDunner(opts, schedulerBillingStore, billingEmailSender, billingDispatcher)
	opts = appendSLACalculator(opts, deps, schedulerBillingStore)
	return opts
}

func appendUsageReportEmailer(
	opts []scheduler.SchedulerOption,
	cfg *config.Config,
	schedulerBillingStore *billing.PgStore,
) []scheduler.SchedulerOption {
	if cfg.ResendAPIKey == "" {
		return opts
	}
	usageReportEmailer := scheduler.NewUsageReportEmailer(
		schedulerBillingStore,
		resend.NewClient(cfg.ResendAPIKey).Emails,
		"billing@strait.dev",
		24*time.Hour,
	)
	opts = append(opts, scheduler.WithUsageReportEmailer(usageReportEmailer))
	slog.Info("usage report emailer enabled")
	return opts
}

func appendDunner(
	opts []scheduler.SchedulerOption,
	schedulerBillingStore *billing.PgStore,
	billingEmailSender *billing.BillingEmailSender,
	billingDispatcher *webhook.BillingDispatcher,
) []scheduler.SchedulerOption {
	dunnerOpts := []billing.DunnerOption{
		billing.WithDunnerEmails(billingEmailSender),
		billing.WithDunnerAdminLookup(schedulerBillingStore),
		billing.WithDunnerLogger(slog.Default()),
	}
	if billingDispatcher != nil {
		dunnerOpts = append(dunnerOpts, billing.WithDunnerDispatcher(billingDispatcher))
	}
	dunner := billing.NewDunner(schedulerBillingStore, dunnerOpts...)
	opts = append(opts, scheduler.WithDunner(dunner))
	slog.Info("dunning state machine enabled")
	return opts
}

func appendSLACalculator(
	opts []scheduler.SchedulerOption,
	deps workerRuntimeDeps,
	schedulerBillingStore *billing.PgStore,
) []scheduler.SchedulerOption {
	cfg := deps.config
	slaCreditStore := billing.NewPgSLACreditStore(deps.dbPool)
	slaCalculator := billing.NewSLACalculator(
		&slaCalculatorStore{PgStore: schedulerBillingStore, slaCreditStore: slaCreditStore},
		newUptimeSource(cfg, slog.Default()),
		24*time.Hour,
		slog.Default(),
	)
	if deps.billingDispatcher != nil {
		slaCalculator.WithDispatcher(deps.billingDispatcher)
	}
	if issuer := newSLAIssuer(cfg, schedulerBillingStore, slog.Default()); issuer != nil {
		slaCalculator.WithIssuer(issuer)
		slog.Info("sla credit stripe issuer enabled")
	}
	opts = append(opts, scheduler.WithSLACalculator(slaCalculator))
	slog.Info("sla credit calculator enabled")
	return opts
}

func appendBackpressureSampler(opts []scheduler.SchedulerOption, deps workerRuntimeDeps) []scheduler.SchedulerOption {
	cfg := deps.config
	if cfg.BackpressureSamplerInterval <= 0 {
		return opts
	}
	qm, err := queue.Metrics()
	if err != nil || qm == nil {
		slog.Warn("backpressure sampler disabled: queue metrics unavailable", "err", err)
		return opts
	}
	if deps.backpressure == nil {
		slog.Warn("backpressure sampler disabled: controller unavailable")
		return opts
	}
	sampler := scheduler.NewBackpressureSampler(
		deps.backpressure,
		qm,
		cfg.BackpressureSamplerInterval,
		cfg.BackpressureSamplerN,
	)
	if sampler == nil {
		return opts
	}
	opts = append(opts, scheduler.WithBackpressureSampler(sampler))
	slog.Info("backpressure sampler enabled",
		"interval", cfg.BackpressureSamplerInterval,
		"sample_n", cfg.BackpressureSamplerN,
	)
	return opts
}

func buildExecutorConfig(
	deps workerRuntimeDeps,
	p *worker.Pool,
	wake <-chan struct{},
	notifier queue.DegradedNotifier,
	adaptive *worker.AdaptiveConcurrency,
) worker.ExecutorConfig {
	cfg := deps.config
	partitions := append([]string(nil), cfg.WorkerPartitions...)
	partitionWeights := cfg.WorkerPartitionWeights
	if len(partitions) > 0 {
		slog.Info("worker queue partitioning enabled", "partitions", partitions)
	}

	execCfg := worker.ExecutorConfig{
		Pool:                     p,
		Queue:                    deps.queue,
		Wake:                     wake,
		Degraded:                 notifier,
		ConcurrencyLimit:         adaptive,
		Store:                    deps.queries,
		TxPool:                   deps.txPool,
		PollInterval:             cfg.PollerInterval,
		HeartbeatInterval:        cfg.HeartbeatInterval,
		Publisher:                deps.publisher,
		Metrics:                  deps.metrics,
		WorkflowCallback:         deps.stepCallback,
		Partitions:               partitions,
		PartitionWeights:         partitionWeights,
		ExecutorHTTPTimeout:      cfg.ExecutorHTTPTimeout,
		ExecutorIdleConnTimeout:  cfg.ExecutorIdleConnTimeout,
		ExecutionTraceMode:       cfg.ExecutionTraceMode,
		AllowPrivateEndpoints:    cfg.AllowPrivateEndpoints,
		WebhookMaxAttempts:       cfg.WebhookMaxAttempts,
		JobCacheTTL:              cfg.JobCacheTTL,
		VersionCacheTTL:          cfg.VersionCacheTTL,
		RunVersionCacheTTL:       cfg.RunVersionCacheTTL,
		JobHealthCacheTTL:        cfg.JobHealthCacheTTL,
		RedisClient:              deps.redisClient,
		CacheRegistry:            deps.cacheRegistry,
		CacheBus:                 deps.cacheBus,
		MaxDequeueBatchSize:      cfg.MaxDequeueBatchSize,
		DefaultJobMaxConcurrency: cfg.DefaultJobMaxConcurrency,
		MaxSnoozeCount:           cfg.MaxSnoozeCount,
		JWTSigningKey:            cfg.JWTSigningKey,
		ExternalAPIURL:           cfg.ExternalAPIURL,
		DefaultRegion:            cfg.DefaultRegion,
		Mode:                     cfg.Mode,
		Version:                  version,
		Edition:                  domain.ParseEdition(cfg.Edition),
		EventChannelSize:         cfg.WorkerEventChannelSize,
		SecretDecryptor:          deps.encryptor,
		DLQCapEnforcer: worker.NewDLQCapEnforcer(deps.queries, worker.DLQCapConfig{
			MaxPerJob:     cfg.DLQMaxPerJob,
			MaxPerProject: cfg.DLQMaxPerProject,
			Policy:        worker.DLQOverflowPolicy(cfg.DLQOverflowPolicy),
		}, slog.Default()),
	}
	applyWorkerPlaneToExecutorConfig(&execCfg, deps.workerPlane, cfg.JWTSigningKey)
	return execCfg
}

func applyWorkerBillingConfig(execCfg *worker.ExecutorConfig, cfg *config.Config, billingEnforcer *billing.Enforcer, metrics *telemetry.Metrics) {
	if cfg.BillingEnforcementEnabled && billingEnforcer != nil {
		execCfg.BillingEnforcer = billingEnforcer
		slog.Info("billing enforcement enabled for worker")
	}

	if cfg.StripeSecretKey == "" {
		return
	}

	execCfg.StripeUsageReporter = billing.NewStripeUsageReporter(
		cfg.StripeSecretKey,
		slog.Default(),
		billing.WithUsageReporterMetrics(metrics),
	)
	slog.Info("stripe usage reporting enabled")
}

func registerWorkerSubscribers(
	exec *worker.Executor,
	pub pubsub.Publisher,
	metrics *telemetry.Metrics,
	chExporter *clickhouse.Exporter,
	queries *store.Queries,
) {
	exec.Use(worker.TracingMiddleware())
	if metrics != nil {
		exec.Subscribe(worker.MetricsSubscriber(metrics))
		exec.Subscribe(worker.PubSubSubscriber(pub, metrics.PubSubPublishErrors))
	} else {
		exec.Subscribe(worker.PubSubSubscriber(pub))
	}
	if chExporter != nil {
		exec.Subscribe(worker.ClickHouseSubscriber(chExporter, queries))
	}
}

func registerWorkerHealthChecks(
	healthReg *health.Registry,
	p *worker.Pool,
	queries *store.Queries,
	workerConcurrency int,
) {
	healthReg.Register(health.NewPoolChecker(p))

	queueDepthThreshold := int64(max(workerConcurrency*100, 1000))
	healthReg.Register(health.NewQueueDepthChecker(func(checkCtx context.Context) (int64, error) {
		stats, err := queries.QueueStats(checkCtx)
		if err != nil {
			return 0, err
		}
		return int64(stats.Queued), nil
	}, queueDepthThreshold))
}

func registerWorkerMetrics(
	g *pool.ContextPool,
	metrics *telemetry.Metrics,
	p *worker.Pool,
	queries *store.Queries,
	chExporter *clickhouse.Exporter,
) {
	if metrics == nil {
		return
	}

	meter := otel.Meter("strait")
	if err := metrics.ObservePool(meter, p); err != nil {
		slog.Warn("failed to register pool metrics callback", "error", err)
	}
	if _, err := meter.RegisterCallback(func(callbackCtx context.Context, observer metric.Observer) error {
		stats, err := queries.QueueStats(callbackCtx)
		if err != nil {
			return fmt.Errorf("load queue stats for queue depth metrics: %w", err)
		}
		observer.ObserveInt64(metrics.QueueDepth, int64(stats.Queued), metric.WithAttributes(attribute.String("status", "queued")))
		observer.ObserveInt64(metrics.QueueDepth, int64(stats.Executing), metric.WithAttributes(attribute.String("status", "executing")))
		observer.ObserveInt64(metrics.QueueDepth, int64(stats.Delayed), metric.WithAttributes(attribute.String("status", "delayed")))
		return nil
	}, metrics.QueueDepth); err != nil {
		slog.Warn("failed to register queue depth metrics callback", "error", err)
	}

	g.Go(func(ctx context.Context) error {
		recordWorkerBacklogMetrics(ctx, metrics, queries, chExporter)
		return nil
	})
}

func recordWorkerBacklogMetrics(
	ctx context.Context,
	metrics *telemetry.Metrics,
	queries *store.Queries,
	chExporter *clickhouse.Exporter,
) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if chExporter != nil {
				metrics.ClickHouseExporterPending.Record(ctx, int64(chExporter.PendingCount()))
			}
			count, err := queries.CountPendingWebhookDeliveries(ctx)
			if err == nil {
				metrics.WebhookBacklogDepth.Record(ctx, count)
			}
		}
	}
}

// anomalyMonitorStore composes the billing store with the notification-channel
// methods on the main queries store so the scheduler's anomaly monitor can
// resolve subscriber orgs (billing) and dispatch notifications (queries) via a
// single dependency.
type anomalyMonitorStore struct {
	*billing.PgStore
	queries *store.Queries
}

func (a *anomalyMonitorStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	return a.queries.ListEnabledNotificationChannels(ctx, projectID)
}

func (a *anomalyMonitorStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	return a.queries.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
}

func (a *anomalyMonitorStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	return a.queries.CreateNotificationDelivery(ctx, d)
}

// slaCalculatorStore composes the billing store (active enterprise contract
// listing) with the dedicated SLA credit store (read/insert) so the
// scheduler's SLA calculator can satisfy a single dependency.
type slaCalculatorStore struct {
	*billing.PgStore
	slaCreditStore *billing.PgSLACreditStore
}

func (s *slaCalculatorStore) InsertSLACredit(ctx context.Context, row billing.SLACreditRow) error {
	return s.slaCreditStore.InsertSLACredit(ctx, row)
}

func (s *slaCalculatorStore) GetSLACredit(ctx context.Context, orgID string, periodStart, periodEnd time.Time) (*billing.SLACreditRow, error) {
	return s.slaCreditStore.GetSLACredit(ctx, orgID, periodStart, periodEnd)
}

func (s *slaCalculatorStore) MarkSLACreditWebhookDispatched(ctx context.Context, orgID string, periodStart, periodEnd, dispatchedAt time.Time) (bool, error) {
	return s.slaCreditStore.MarkSLACreditWebhookDispatched(ctx, orgID, periodStart, periodEnd, dispatchedAt)
}

func applyWorkerPlaneToExecutorConfig(execCfg *worker.ExecutorConfig, workerPlane *grpcserver.Server, jwtSigningKey string) {
	if execCfg == nil || workerPlane == nil {
		return
	}
	execCfg.QueueSnapshotter = workerPlane.Registry()
	execCfg.WorkerDispatcher = workerPlane.WorkerDispatcher(jwtSigningKey)
}

func runMigrations(databaseURL, mode string, lockTimeout time.Duration) error {
	switch mode {
	case "manual":
		slog.Info("migration mode is manual, skipping migrations")
		return nil
	case "validate", "auto":
		// continue below
	default:
		return fmt.Errorf("unknown migration mode: %s", mode)
	}

	// Use pgx/v5/stdlib shim for database/sql compatibility
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open sql connection: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer conn.Close()

	// Set lock_timeout on the same connection used by golang-migrate's
	// PostgreSQL driver. A pool-level SET can land on a different connection
	// and leave migration DDL waiting indefinitely on locks.
	if lockTimeout > 0 {
		lockTimeoutMs := lockTimeout.Milliseconds()
		if _, execErr := conn.ExecContext(ctx, fmt.Sprintf("SET lock_timeout = '%dms'", lockTimeoutMs)); execErr != nil {
			return fmt.Errorf("set lock_timeout: %w", execErr)
		}
		slog.Info("migration lock timeout set", "timeout_ms", lockTimeoutMs)
	}

	driver, err := pgmigrate.WithConnection(ctx, conn, &pgmigrate.Config{
		MigrationsTable: "strait_schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	version, dirty, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("check migration version: %w", vErr)
	}
	slog.Info("current migration state", "version", version, "dirty", dirty)

	if dirty {
		return fmt.Errorf("database has dirty migration at version %d; resolve manually", version)
	}

	if mode == "validate" {
		slog.Info("migration mode is validate, skipping apply")
		return nil
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	slog.Info("migrations applied")
	return nil
}

func startMaintenanceWorker(g *pool.ContextPool, queries *store.Queries) {
	g.Go(func(ctx context.Context) error {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if n, err := queries.CleanExpiredIdempotencyKeys(ctx); err != nil {
					slog.Warn("failed to clean expired idempotency keys", "error", err)
				} else if n > 0 {
					slog.Info("cleaned expired idempotency keys", "count", n)
				}
			}
		}
	})
}

// startQueueHealthSampler reads pg_stat_user_tables for job_runs partitions
// every 30s and publishes live/
// dead tuple counts, HOT ratio, and oldest queued age gauges.
func startQueueHealthSampler(g *pool.ContextPool, dbPool *pgxpool.Pool) {
	sampler, err := queue.NewHealthSampler(dbPool, 30*time.Second, slog.Default())
	if err != nil {
		slog.Warn("failed to create queue health sampler, skipping", "error", err)
		return
	}
	g.Go(func(ctx context.Context) error {
		slog.Info("queue health sampler started")
		sampler.Run(ctx)
		return nil
	})
}

// startDBPoolSampler launches a goroutine that samples pgxpool connection
// statistics every 15s and records them as OTel gauges.
func startDBPoolSampler(g *pool.ContextPool, dbPool *pgxpool.Pool) {
	sampler, err := telemetry.NewPoolSampler(dbPool, 15*time.Second, slog.Default())
	if err != nil {
		slog.Warn("failed to create db pool sampler, skipping", "error", err)
		return
	}
	g.Go(func(ctx context.Context) error {
		slog.Info("db pool sampler started")
		sampler.Run(ctx)
		return nil
	})
}

// startDBWatchdog launches the MVCC-horizon watchdog.
func startDBWatchdog(g *pool.ContextPool, cfg *config.Config, dbPool *pgxpool.Pool) {
	if !cfg.DBWatchdogEnabled {
		slog.Info("db watchdog disabled via config")
		return
	}
	watchdog, err := telemetry.NewDBWatchdog(dbPool, cfg.DBWatchdogInterval, cfg.DBLongTxnAlertThreshold, slog.Default())
	if err != nil {
		slog.Warn("failed to create db watchdog, skipping", "error", err)
		return
	}
	g.Go(func(ctx context.Context) error {
		slog.Info("db watchdog started",
			"interval", cfg.DBWatchdogInterval,
			"alert_threshold", cfg.DBLongTxnAlertThreshold,
		)
		watchdog.Run(ctx)
		return nil
	})
}

// nilSafeBillingEnforcer prevents the classic Go nil-interface trap where a
// typed nil (*billing.Enforcer)(nil) assigned to an interface makes the
// interface value non-nil, causing nil-pointer dereferences on method calls.
func nilSafeBillingEnforcer(e *billing.Enforcer) api.BillingEnforcer {
	if e == nil {
		return nil
	}
	return e
}
