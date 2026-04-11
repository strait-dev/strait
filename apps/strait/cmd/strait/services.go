package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"strait/internal/agents"
	"strait/internal/api"
	"strait/internal/billing"
	"strait/internal/build"
	"strait/internal/cdc"
	"strait/internal/clickhouse"
	"strait/internal/compute"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/logdrain"
	"strait/internal/notification"
	"strait/internal/objectstore"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/registry"
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
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc/pool"
)

// validRegistryHostnameRe matches a Docker-style registry host[:port] key used in
// BUILD_EXTRA_REGISTRY_AUTHS. Allows letters, digits, dots, hyphens, and an optional
// colon-separated port. Rejects embedded credentials, path-traversal sequences, and
// protocol prefixes that could be used to redirect auth to an attacker-controlled host.
var validRegistryHostnameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*(:[0-9]{1,5})?$`)

// isPrivateRegistryHost reports whether host is a loopback address, a private or
// link-local IP, or the "localhost" name. Such hosts are rejected from
// BUILD_EXTRA_REGISTRY_AUTHS because a user-supplied bearer token sent to a
// localhost registry would be forwarded inside the build worker, giving an
// attacker-controlled base-image server the ability to harvest registry credentials
// via SSRF.
func isPrivateRegistryHost(host string) bool {
	// Strip optional port before checking.
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		// No port — treat the whole string as the host.
		h = host
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

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

// buildComputeRuntime constructs the container runtime based on config.
// If a fallback provider is configured, wraps both in a RuntimeRouter.
func buildComputeRuntime(cfg *config.Config, metrics *telemetry.Metrics) compute.ContainerRuntime {
	primary := buildSingleRuntime(cfg.ComputeRuntime, cfg, metrics)
	if primary == nil {
		return nil
	}

	if cfg.ComputeFallbackProvider == "" {
		return primary
	}

	fallback := buildSingleRuntime(cfg.ComputeFallbackProvider, cfg, metrics)
	if fallback == nil {
		slog.Warn("fallback runtime failed to initialize, running without fallback",
			"primary", cfg.ComputeRuntime,
			"fallback", cfg.ComputeFallbackProvider,
		)
		return primary
	}

	slog.Info("compute runtime with fallback enabled",
		"primary", cfg.ComputeRuntime,
		"fallback", cfg.ComputeFallbackProvider,
	)
	return compute.NewRuntimeRouter(primary, fallback)
}

func buildSingleRuntime(provider string, cfg *config.Config, metrics *telemetry.Metrics) compute.ContainerRuntime {
	switch provider {
	case "docker":
		slog.Info("container runtime enabled", "runtime", "docker")
		return compute.NewDockerRuntime()
	case "k8s":
		rt, err := compute.NewK8sRuntime(cfg.K8sKubeconfig, cfg.K8sNamespace, cfg.K8sPriorityClass, cfg.ImagePullPolicy)
		if err != nil {
			slog.Error("CRITICAL: k8s runtime init failed", "error", err)
			return nil
		}
		rt.SetMetrics(telemetry.NewK8sMetricsAdapter(metrics))
		if cfg.K8sRuntimeClass != "" {
			rt.SetRuntimeClass(cfg.K8sRuntimeClass)
			slog.Info("container runtime enabled", "runtime", "k8s", "namespace", cfg.K8sNamespace, "runtime_class", cfg.K8sRuntimeClass)
		} else {
			slog.Info("container runtime enabled", "runtime", "k8s", "namespace", cfg.K8sNamespace)
		}
		return rt
	default:
		return nil
	}
}

// connectDatabase creates and verifies a Postgres connection pool.
// It retries with exponential backoff up to 5 times on transient failures.
func connectDatabase(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if cfg.DBPgBouncerMode {
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	if cfg.DBHealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = cfg.DBHealthCheckPeriod
	}
	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())

	// Apply statement_timeout to the API connection pool to prevent runaway queries.
	if cfg.DBStatementTimeout > 0 {
		if poolConfig.ConnConfig.RuntimeParams == nil {
			poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
		}
		poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", cfg.DBStatementTimeout.Milliseconds())
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

// connectRedis creates and verifies a Redis client for pub/sub. Returns a nil
// publisher and client when Redis is not configured.
// It retries with exponential backoff up to 5 times on transient failures.
func connectRedis(ctx context.Context, cfg *config.Config) (pubsub.Publisher, *redis.Client, error) {
	rdb, err := pubsub.NewRedisClient(cfg.RedisURL, cfg.RedisSentinelMaster, cfg.RedisSentinelAddrs)
	if err != nil {
		return nil, nil, fmt.Errorf("create redis client: %w", err)
	}
	if rdb == nil {
		return nil, nil, nil
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
		pub := pubsub.NewRedisPublisher(rdb)
		return pub, rdb, nil
	}

	return nil, nil, fmt.Errorf("ping redis: failed after %d retries", maxRetries)
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

// startCDCConsumer registers and starts the Sequin CDC consumer if configured.
// Returns a webhook receiver for push-based CDC delivery (nil if CDC is disabled).
func startCDCConsumer(g *pool.ContextPool, cfg *config.Config, pub pubsub.Publisher, queries *store.Queries, chExporter *clickhouse.Exporter) *cdc.WebhookReceiver {
	if cfg.SequinBaseURL == "" {
		return nil
	}

	cdcClient := cdc.NewClient(cfg.SequinBaseURL, cfg.SequinConsumerName, cfg.SequinAPIToken)

	// Auto-provision the Sequin consumer if it does not exist.
	cdcTables := []string{
		"public.job_runs", "public.workflow_runs",
		"public.workflow_step_runs", "public.event_triggers",
	}
	if err := cdcClient.EnsureConsumer(context.Background(), cdcTables); err != nil {
		slog.Warn("failed to auto-provision Sequin consumer, CDC may not work",
			"error", err, "consumer", cfg.SequinConsumerName)
	}

	cdcConsumer := cdc.NewConsumer(cdcClient, cdc.ConsumerConfig{
		BaseURL:      cfg.SequinBaseURL,
		ConsumerName: cfg.SequinConsumerName,
		Credential:   cfg.SequinAPIToken,
		BatchSize:    cfg.CDCBatchSize,
		WaitTimeMs:   cfg.CDCWaitTimeMs,
	}, slog.Default())

	cdcConsumer.RegisterHandler(cdc.NewJobRunHandler(pub, slog.Default()))
	cdcConsumer.RegisterHandler(cdc.NewWorkflowRunHandler(pub, slog.Default()))
	cdcConsumer.RegisterHandler(cdc.NewWorkflowStepRunHandler(pub, slog.Default()))
	cdcConsumer.RegisterHandler(cdc.NewEventTriggerHandler(pub, slog.Default()))

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
	webhookReceiver := cdc.NewWebhookReceiver(pub, slog.Default())
	webhookReceiver.RegisterHandler(cdc.NewJobRunHandler(pub, slog.Default()))
	webhookReceiver.RegisterHandler(cdc.NewWorkflowRunHandler(pub, slog.Default()))
	webhookReceiver.RegisterHandler(cdc.NewWorkflowStepRunHandler(pub, slog.Default()))
	webhookReceiver.RegisterHandler(cdc.NewEventTriggerHandler(pub, slog.Default()))

	// CDC-driven side effects: each handler watches job_runs for status
	// transitions and triggers a downstream action. Registered as additional
	// handlers since the pull consumer only supports one handler per table.
	webhookReceiver.RegisterAdditionalHandler(cdc.NewWebhookTriggerHandler(queries, slog.Default()))
	webhookReceiver.RegisterAdditionalHandler(cdc.NewNotificationTriggerHandler(queries, slog.Default()))
	webhookReceiver.RegisterAdditionalHandler(cdc.NewAuditHandler(queries, slog.Default()))
	webhookReceiver.RegisterAdditionalHandler(cdc.NewSLOHandler(queries, slog.Default()))
	if chExporter != nil {
		webhookReceiver.RegisterAdditionalHandler(cdc.NewAnalyticsHandler(chExporter, slog.Default()))
	}

	slog.Info("cdc consumer enabled",
		"base_url", cfg.SequinBaseURL,
		"consumer", cfg.SequinConsumerName,
	)

	return webhookReceiver
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

	httpClient := &http.Client{Timeout: 15 * time.Second}
	notifWorker := notification.NewWorker(queries, httpClient)
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

// startAPIServer starts the HTTP API server and its graceful shutdown goroutine.
func startAPIServer(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, txPool store.TxBeginner, dbPool *pgxpool.Pool, q *queue.PostgresQueue, pub pubsub.Publisher, metricsHandler http.Handler, metrics *telemetry.Metrics, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine, healthReg *health.Registry, rdb *redis.Client, encryptor api.Encryptor, billingEnforcer *billing.Enforcer, analyticsStore api.AnalyticsStore, chExporter *clickhouse.Exporter, cdcWebhookReceiver *cdc.WebhookReceiver) {
	if cfg.Mode != "api" && cfg.Mode != "all" {
		return
	}

	var pinger api.Pinger
	if redisPub, ok := pub.(*pubsub.RedisPublisher); ok {
		pinger = redisPub
	}

	if pinger != nil {
		healthReg.Register(health.NewCriticalChecker("redis", false, func(ctx context.Context) error {
			return pinger.Ping(ctx)
		}))
	}
	if cfg.SequinBaseURL != "" {
		sequinHealthURL := cfg.SequinBaseURL + "/health"
		healthReg.Register(health.NewCriticalChecker("sequin_cdc", false, func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, sequinHealthURL, nil)
			if err != nil {
				return fmt.Errorf("sequin health request: %w", err)
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("sequin unreachable: %w", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 500 {
				return fmt.Errorf("sequin unhealthy: HTTP %d", resp.StatusCode)
			}
			return nil
		}))
	}

	apiContainerRuntime := buildComputeRuntime(cfg, metrics)

	billingStore := billing.NewPgStore(dbPool)

	posthogClient := billing.NewPostHogClient(cfg.PostHogAPIKey, cfg.PostHogHost, slog.Default())

	var stripeWebhook http.Handler
	if cfg.StripeWebhookSecret != "" {
		stripeMapping := billing.NewStripeMappingFromOptions(
			billing.WithStarterPrices(cfg.StripeStarterMonthlyPriceID, cfg.StripeStarterYearlyPriceID),
			billing.WithProPrices(cfg.StripeProMonthlyPriceID, cfg.StripeProYearlyPriceID),
			billing.WithScalePrices(cfg.StripeScaleMonthlyPriceID, cfg.StripeScaleYearlyPriceID),
			billing.WithEnterpriseStarterPrice(cfg.StripeEnterpriseStarterYearlyPriceID),
			billing.WithEnterpriseGrowthPrice(cfg.StripeEnterpriseGrowthYearlyPriceID),
			billing.WithEnterpriseLargePrice(cfg.StripeEnterpriseLargeYearlyPriceID),
			billing.WithAddonPrice(cfg.StripeAddonConcurrentRunsID, billing.AddonConcurrentRuns),
			billing.WithAddonPrice(cfg.StripeAddonMembersID, billing.AddonMembers),
			billing.WithAddonPrice(cfg.StripeAddonCronSchedulesID, billing.AddonCronSchedules),
			billing.WithAddonPrice(cfg.StripeAddonDataRetentionID, billing.AddonDataRetention),
			billing.WithAddonPrice(cfg.StripeAddonWebhookEndpointsID, billing.AddonWebhookEndpoints),
			// Agent plan prices
			billing.WithAgentMakerPrices(cfg.StripeAgentMakerMonthlyPriceID, cfg.StripeAgentMakerYearlyPriceID),
			billing.WithAgentGrowthPrices(cfg.StripeAgentGrowthMonthlyPriceID, cfg.StripeAgentGrowthYearlyPriceID),
			// Agent add-on prices
			billing.WithAddonPrice(cfg.StripeAgentAddonConcurrentRunsID, billing.AddonAgentConcurrentRuns),
			billing.WithAddonPrice(cfg.StripeAgentAddonDefinitionsID, billing.AddonAgentDefinitions),
			billing.WithAddonPrice(cfg.StripeAgentAddonMemoryID, billing.AddonAgentMemory),
			billing.WithAddonPrice(cfg.StripeAgentAddonRetentionID, billing.AddonAgentRetention),
			billing.WithAddonPrice(cfg.StripeAgentAddonWebhookEndpointsID, billing.AddonAgentWebhookEndpoints),
		)
		var webhookOpts []billing.WebhookOption
		if posthogClient != nil {
			webhookOpts = append(webhookOpts, billing.WithPostHog(posthogClient))
		}
		if cfg.ResendAPIKey != "" {
			resendClient := billing.NewResendWelcomeEmailFunc(cfg.ResendAPIKey, cfg.ResendFromEmail)
			webhookOpts = append(webhookOpts, billing.WithWelcomeEmail(resendClient))
		}
		billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default())
		if billingEmailSender != nil {
			webhookOpts = append(webhookOpts, billing.WithBillingEmails(billingEmailSender))
		}
		webhookOpts = append(webhookOpts, billing.WithEdition(cfg.Edition))
		wh := billing.NewWebhookHandler(billingStore, stripeMapping, cfg.StripeWebhookSecret, slog.Default(), billingEnforcer, queries, webhookOpts...)
		g.Go(func(ctx context.Context) error {
			wh.StartReplayCleanup(ctx)
			<-ctx.Done()
			return nil
		})
		stripeWebhook = wh
		slog.Info("stripe webhook handler enabled")
	}

	var usageSvc *billing.UsageService
	if billingEnforcer != nil {
		usageSvc = billing.NewUsageService(billingStore, billingEnforcer)
	}

	// Wire the agent service — enables /v1/agents endpoints.
	cfCfg := agents.CloudflareConfig{
		AccountID:                cfg.CFAccountID,
		APIToken:                 cfg.CFAPIToken,
		DispatchNamespace:        cfg.CFDispatchNamespace,
		DispatchNamespaceStaging: cfg.CFDispatchNamespaceStaging,
		DispatchWorkerURL:        cfg.CFDispatchWorkerURL,
		OutboundWorkerName:       cfg.CFOutboundWorkerName,
		CompatibilityDate:        cfg.CFCompatibilityDate,
		SandboxMode:              agents.CloudflareSandboxMode(cfg.CFSandboxMode),
	}
	agentOpts := []agents.Option{
		agents.WithProvider(agents.SelectProvider(cfCfg)),
		agents.WithJWTSigningKey(cfg.JWTSigningKey),
		agents.WithInternalSecret(cfg.InternalSecret),
		agents.WithDispatchHTTPClient(&http.Client{Timeout: 30 * time.Second}),
		agents.WithAPIBaseURL(fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)),
		agents.WithQuotaChecker(queries),
		agents.WithWebhookStore(queries),
	}
	if billingEnforcer != nil {
		agentOpts = append(agentOpts, agents.WithBillingEnforcer(billingEnforcer))
	}
	agentSvc := agents.NewService(queries, txPool, agentOpts...)
	slog.Info("agent service initialized", "cf_enabled", cfCfg.Enabled())

	var doMemClient *agents.DOMemoryClient
	if cfCfg.Enabled() {
		doMemClient = agents.NewDOMemoryClient(cfg.CFAccountID, cfg.CFDispatchNamespace, cfg.CFAPIToken)
	}

	var agentBilling *billing.AgentBillingReporter
	if cfg.StripeSecretKey != "" {
		agentBilling = billing.NewAgentBillingReporter(
			cfg.StripeSecretKey, slog.Default(),
			billing.WithUsageReporterMetrics(metrics),
		)
		slog.Info("agent billing enabled")
	}

	srv := api.NewServer(api.ServerDeps{
		Config:             cfg,
		Store:              queries,
		AgentService:       agentSvc,
		AnalyticsStore:     analyticsStore,
		Queue:              q,
		PubSub:             pub,
		MetricsHandler:     metricsHandler,
		Metrics:            metrics,
		Pinger:             pinger,
		HealthRegistry:     healthReg,
		WorkflowCallback:   stepCallback,
		WorkflowEngine:     workflowEngine,
		TxPool:             txPool,
		RedisClient:        rdb,
		Encryptor:          encryptor,
		ContainerRuntime:   apiContainerRuntime,
		StripeWebhook:      stripeWebhook,
		BillingEnforcer:    nilSafeBillingEnforcer(billingEnforcer),
		UsageService:       usageSvc,
		CHExporter:         chExporter,
		AgentBilling:       agentBilling,
		DOMemoryClient:     doMemClient,
		Edition:            domain.ParseEdition(cfg.Edition),
		Version:            version,
		CDCWebhookReceiver: cdcWebhookReceiver,
		ObjectStore:        buildObjectStore(cfg),
	})
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
		err := httpServer.Shutdown(shutdownCtx)
		agentSvc.Close() // close dispatch pool after server stops accepting requests
		return err
	})
}

// startWorker starts the job executor, worker pool, and scheduler goroutines.
func startWorker(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, txPool store.TxBeginner, dbPool *pgxpool.Pool, q *queue.PostgresQueue, pub pubsub.Publisher, metrics *telemetry.Metrics, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine, healthReg *health.Registry, billingEnforcer *billing.Enforcer, chExporter *clickhouse.Exporter) {
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

	partitions := []string(nil)
	partitionWeights := ""
	if len(cfg.WorkerPartitions) > 0 {
		partitions = cfg.WorkerPartitions
		partitionWeights = cfg.WorkerPartitionWeights
		slog.Info("worker queue partitioning enabled", "partitions", partitions)
	}
	containerRuntime := buildComputeRuntime(cfg, metrics)

	// Start K8s job garbage collector if using K8s runtime.
	if (cfg.ComputeRuntime == "k8s" || cfg.ComputeFallbackProvider == "k8s") && cfg.K8sGCEnabled {
		if k8sRT, ok := containerRuntime.(*compute.K8sRuntime); ok {
			gc := compute.NewK8sJobGC(k8sRT.Clientset(), cfg.K8sNamespace, cfg.K8sGCMaxAge, cfg.K8sGCInterval)
			g.Go(func(ctx context.Context) error {
				gc.Run(ctx)
				return nil
			})
			slog.Info("k8s job GC enabled", "max_age", cfg.K8sGCMaxAge, "interval", cfg.K8sGCInterval)
		} else if router, ok := containerRuntime.(*compute.RuntimeRouter); ok {
			_ = router // GC runs against whichever runtime is K8s — extract clientset from primary or fallback
			clientset, err := compute.BuildK8sClientset(cfg.K8sKubeconfig)
			if err != nil {
				slog.Error("failed to build k8s client for GC", "error", err)
			} else {
				gc := compute.NewK8sJobGC(clientset, cfg.K8sNamespace, cfg.K8sGCMaxAge, cfg.K8sGCInterval)
				g.Go(func(ctx context.Context) error {
					gc.Run(ctx)
					return nil
				})
				slog.Info("k8s job GC enabled (via router)", "max_age", cfg.K8sGCMaxAge, "interval", cfg.K8sGCInterval)
			}
		}
	}

	execCfg := worker.ExecutorConfig{
		Pool:                    p,
		Queue:                   q,
		Wake:                    wake,
		ConcurrencyLimit:        adaptive,
		Store:                   queries,
		TxPool:                  txPool,
		PollInterval:            cfg.PollerInterval,
		HeartbeatInterval:       cfg.HeartbeatInterval,
		Publisher:               pub,
		Metrics:                 metrics,
		WorkflowCallback:        stepCallback,
		Partitions:              partitions,
		PartitionWeights:        partitionWeights,
		ExecutorHTTPTimeout:     cfg.ExecutorHTTPTimeout,
		ExecutorIdleConnTimeout: cfg.ExecutorIdleConnTimeout,
		WebhookMaxAttempts:      cfg.WebhookMaxAttempts,
		MaxSnoozeCount:          cfg.MaxSnoozeCount,
		JWTSigningKey:           cfg.JWTSigningKey,
		DequeueStrategy:         cfg.DequeueStrategy,
		ContainerRuntime:        containerRuntime,
		ExternalAPIURL:          cfg.ExternalAPIURL,
		MaxConcurrentMachines:   cfg.MaxConcurrentMachines,
		DefaultRegion:           cfg.DefaultRegion,
	}

	// Only wire billing enforcement in the executor when explicitly enabled.
	// The enforcer may exist for webhook cache invalidation without executor enforcement.
	if cfg.BillingEnforcementEnabled && billingEnforcer != nil {
		execCfg.BillingEnforcer = billingEnforcer
		slog.Info("billing enforcement enabled")
	}

	// Wire Stripe usage event reporting for metered billing (cloud only).
	if cfg.StripeSecretKey != "" {
		execCfg.StripeUsageReporter = billing.NewStripeUsageReporter(
			cfg.StripeSecretKey, slog.Default(),
			billing.WithUsageReporterMetrics(metrics),
		)
		slog.Info("stripe usage reporting enabled")
	}

	exec := worker.NewExecutor(execCfg)

	exec.Use(worker.TracingMiddleware())
	if metrics != nil {
		exec.Subscribe(worker.MetricsSubscriber(metrics))
	}
	if pub != nil {
		if metrics != nil {
			exec.Subscribe(worker.PubSubSubscriber(pub, metrics.PubSubPublishErrors))
		} else {
			exec.Subscribe(worker.PubSubSubscriber(pub))
		}
	}
	if chExporter != nil {
		exec.Subscribe(worker.ClickHouseSubscriber(chExporter, queries, worker.ClickHouseSubscriberDeps{
			UsageLister:        queries,
			ComputeUsageLister: queries,
		}))
	}

	healthReg.Register(health.NewPoolChecker(p))

	queueDepthThreshold := int64(max(cfg.WorkerConcurrency*100, 1000))
	healthReg.Register(health.NewQueueDepthChecker(func(checkCtx context.Context) (int64, error) {
		stats, err := queries.QueueStats(checkCtx)
		if err != nil {
			return 0, err
		}
		return int64(stats.Queued), nil
	}, queueDepthThreshold))

	if metrics != nil {
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

		// Report webhook backlog and ClickHouse exporter buffer depth.
		g.Go(func(ctx context.Context) error {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
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
		})
	}

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

	// Start build orchestrator for code-first deployments.
	startBuildOrchestrator(g, cfg, queries, pub, metrics)

	// Start scheduler (cron, delayed poller, reaper)
	g.Go(func(ctx context.Context) error {
		budgetWebhookAdapter := scheduler.NewBudgetWebhookAdapter(queries)
		schedOpts := []scheduler.SchedulerOption{
			scheduler.WithSchedulerMetrics(metrics),
			scheduler.WithBudgetWebhookEnqueuer(budgetWebhookAdapter),
			scheduler.WithChExporter(chExporter),
			scheduler.WithIndexMaintainerAdvisoryLocker(queries),
		}
		if containerRuntime != nil {
			schedOpts = append(schedOpts, scheduler.WithMachineStopper(containerRuntime))
		}
		if cfg.BillingEnforcementEnabled && billingEnforcer != nil {
			reconciler := scheduler.NewConcurrentReconciler(billingEnforcer, queries, 5*time.Minute)
			schedOpts = append(schedOpts, scheduler.WithConcurrentReconciler(reconciler))
			slog.Info("concurrent run reconciler enabled")

			billingStore := billing.NewPgStore(dbPool)
			downgradeApplier := scheduler.NewDowngradeApplier(billingStore, billingEnforcer, 5*time.Minute)
			schedOpts = append(schedOpts, scheduler.WithDowngradeApplier(downgradeApplier))
			slog.Info("downgrade applier enabled")

			webhookCleanup := scheduler.NewWebhookMessageCleanup(billingStore, slog.Default())
			schedOpts = append(schedOpts, scheduler.WithWebhookMessageCleanup(webhookCleanup))

			billingEmailSender := billing.NewBillingEmailSender(cfg.ResendAPIKey, "billing@strait.dev", slog.Default())
			contractExpiryChecker := scheduler.NewContractExpiryChecker(billingStore, billingEmailSender, 24*time.Hour)
			schedOpts = append(schedOpts, scheduler.WithContractExpiryChecker(contractExpiryChecker))
			slog.Info("contract expiry checker enabled")

			retentionResolver := billing.NewPlanRetentionResolver(billingStore)
			schedOpts = append(schedOpts, scheduler.WithOrgRetentionResolver(retentionResolver))
			slog.Info("per-org plan retention enabled")
		}
		sched := scheduler.New(ctx, cfg, queries, q, stepCallback, workflowEngine,
			schedOpts...,
		)
		if err := sched.Start(ctx); err != nil {
			return fmt.Errorf("start scheduler: %w", err)
		}
		<-ctx.Done()
		slog.Info("shutting down scheduler")
		sched.Stop()
		return nil
	})
}

// buildObjectStore constructs an object store from config, or returns nil if
// object store is not configured (bucket is empty).
func buildObjectStore(cfg *config.Config) objectstore.ObjectStore {
	if cfg.ObjectStoreBucket == "" {
		return nil
	}
	s, err := objectstore.NewS3Store(objectstore.S3StoreConfig{
		Bucket:         cfg.ObjectStoreBucket,
		Region:         cfg.ObjectStoreRegion,
		Endpoint:       cfg.ObjectStoreEndpoint,
		AccessKey:      cfg.ObjectStoreAccessKey,
		SecretKey:      cfg.ObjectStoreSecretKey,
		ForcePathStyle: cfg.ObjectStoreForcePathStyle,
	})
	if err != nil {
		slog.Warn("object store configuration invalid, code-first deployments disabled", "error", err)
		return nil
	}
	return s
}

// buildContainerRegistry constructs a container registry from config, or returns
// nil if no registry is configured.
func buildContainerRegistry(cfg *config.Config) registry.ContainerRegistry {
	ctx := context.Background()
	switch cfg.ContainerRegistryType {
	case "ecr", "":
		reg, err := registry.NewECRRegistry(ctx, registry.ECRConfig{
			Region:           cfg.ObjectStoreRegion,
			RepositoryPrefix: cfg.ContainerRegistryPrefix,
		})
		if err != nil {
			slog.Warn("ECR registry configuration invalid, code-first deployments disabled", "error", err)
			return nil
		}
		return reg
	case "generic":
		if cfg.ContainerRegistryURL == "" {
			slog.Warn("CONTAINER_REGISTRY_URL not set for generic registry, code-first deployments disabled")
			return nil
		}
		reg, err := registry.NewGenericRegistry(registry.GenericConfig{
			RegistryURL:      cfg.ContainerRegistryURL,
			Username:         cfg.ContainerRegistryUser,
			Password:         cfg.ContainerRegistryPass,
			RepositoryPrefix: cfg.ContainerRegistryPrefix,
		})
		if err != nil {
			slog.Warn("generic registry configuration invalid, code-first deployments disabled", "error", err)
			return nil
		}
		return reg
	default:
		slog.Warn("unknown CONTAINER_REGISTRY_TYPE, code-first deployments disabled",
			"type", cfg.ContainerRegistryType)
		return nil
	}
}

// startBuildOrchestrator starts the build orchestrator that picks up deployments
// in "building" status and executes the BuildKit build pipeline.
// No-ops if the worker mode is not enabled or if object store / registry are not configured.
func startBuildOrchestrator(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, pub pubsub.Publisher, metrics *telemetry.Metrics) {
	if cfg.Mode != "worker" && cfg.Mode != "all" {
		return
	}

	objStore := buildObjectStore(cfg)
	reg := buildContainerRegistry(cfg)

	if objStore == nil || reg == nil {
		slog.Info("build orchestrator disabled (object store or registry not configured)")
		return
	}

	builder := build.NewBuilder(
		cfg.BuildKitAddress,
		objStore,
		reg,
		cfg.BuildKitCacheEnabled,
		cfg.BuildTimeout,
	)
	if pub != nil {
		builder = builder.WithLogPublisher(pub)
	}
	if cfg.SOCIEnabled {
		builder = builder.WithSOCI(true)
		slog.Info("soci lazy image loading enabled")
	}
	if cfg.BuildExtraRegistryAuths != "" && cfg.BuildExtraRegistryAuths != "{}" {
		var extraAuths map[string]string
		if err := json.Unmarshal([]byte(cfg.BuildExtraRegistryAuths), &extraAuths); err != nil {
			slog.Warn("failed to parse BUILD_EXTRA_REGISTRY_AUTHS, skipping extra registry auth", "error", err)
		} else {
			// Validate each hostname key before passing to the builder. Reject entries
			// whose host contains protocol prefixes, path-traversal sequences, embedded
			// credentials, or other patterns that could redirect auth to a rogue registry.
			for host := range extraAuths {
				if !validRegistryHostnameRe.MatchString(host) || isPrivateRegistryHost(host) {
					slog.Warn("BUILD_EXTRA_REGISTRY_AUTHS: skipping entry with invalid hostname", "host", host)
					delete(extraAuths, host)
				}
			}
			if len(extraAuths) > 0 {
				builder = builder.WithExtraRegistryAuths(extraAuths)
			}
		}
	}
	addrPool := build.NewAddressPool(cfg.BuildKitAddress, cfg.BuildKitAddresses)
	orchOpts := []build.OrchestratorOption{
		build.WithOrchestratorLogger(slog.Default()),
		build.WithAddressPool(addrPool),
		build.WithOrchestratorMetrics(metrics),
	}
	orch := build.NewOrchestrator(queries, builder, orchOpts...)

	g.Go(func(ctx context.Context) error {
		slog.Info("build orchestrator started",
			"buildkit_addresses", addrPool.Len(),
			"buildkit_addr", cfg.BuildKitAddress,
		)
		orch.Run(ctx)
		return nil
	})

	if cfg.DeploymentGCEnabled {
		gc := build.NewDeploymentGC(queries,
			build.WithGCInterval(cfg.DeploymentGCInterval),
			build.WithGCPendingTTL(cfg.DeploymentGCPendingTTL),
			build.WithGCFailedAge(cfg.DeploymentGCFailedAge),
			build.WithGCLogger(slog.Default()),
			build.WithGCMetrics(metrics),
		)
		g.Go(func(ctx context.Context) error {
			slog.Info("deployment GC started",
				"interval", cfg.DeploymentGCInterval,
				"pending_ttl", cfg.DeploymentGCPendingTTL,
				"failed_age", cfg.DeploymentGCFailedAge,
			)
			gc.Run(ctx)
			return nil
		})
	}
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

	driver, err := pgmigrate.WithInstance(db, &pgmigrate.Config{
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

	// Set lock timeout to prevent long DDL waits blocking other transactions.
	if lockTimeout > 0 {
		lockTimeoutMs := lockTimeout.Milliseconds()
		if _, execErr := db.Exec(fmt.Sprintf("SET lock_timeout = '%dms'", lockTimeoutMs)); execErr != nil {
			return fmt.Errorf("set lock_timeout: %w", execErr)
		}
		slog.Info("migration lock timeout set", "timeout_ms", lockTimeoutMs)
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

// nilSafeBillingEnforcer prevents the classic Go nil-interface trap where a
// typed nil (*billing.Enforcer)(nil) assigned to an interface makes the
// interface value non-nil, causing nil-pointer dereferences on method calls.
func nilSafeBillingEnforcer(e *billing.Enforcer) api.BillingEnforcer {
	if e == nil {
		return nil
	}
	return e
}

// wrapUsageService prevents the nil-interface trap for UsageService.
func wrapUsageService(s *billing.UsageService) api.UsageService {
	if s == nil {
		return nil
	}
	return s
}
