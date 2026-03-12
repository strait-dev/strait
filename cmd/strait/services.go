package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"strait/internal/api"
	"strait/internal/cdc"
	"strait/internal/config"
	"strait/internal/health"
	"strait/internal/pubsub"
	"strait/internal/queue"
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
	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())

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
func startCDCConsumer(g *pool.ContextPool, cfg *config.Config, pub pubsub.Publisher) {
	if cfg.SequinBaseURL == "" {
		return
	}

	cdcClient := cdc.NewClient(cfg.SequinBaseURL, cfg.SequinConsumerName, cfg.SequinAPIToken)
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

	slog.Info("cdc consumer enabled",
		"base_url", cfg.SequinBaseURL,
		"consumer", cfg.SequinConsumerName,
	)
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

// startAPIServer starts the HTTP API server and its graceful shutdown goroutine.
func startAPIServer(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, txPool store.TxBeginner, q *queue.PostgresQueue, pub pubsub.Publisher, metricsHandler http.Handler, metrics *telemetry.Metrics, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine, healthReg *health.Registry) {
	if cfg.Mode != "api" && cfg.Mode != "all" {
		return
	}

	var pinger api.Pinger
	if redisPub, ok := pub.(*pubsub.RedisPublisher); ok {
		pinger = redisPub
	}

	if pinger != nil {
		healthReg.Register(health.NewChecker("redis", func(ctx context.Context) error {
			return pinger.Ping(ctx)
		}))
	}

	srv := api.NewServer(api.ServerDeps{
		Config:           cfg,
		Store:            queries,
		Queue:            q,
		PubSub:           pub,
		MetricsHandler:   metricsHandler,
		Metrics:          metrics,
		Pinger:           pinger,
		HealthRegistry:   healthReg,
		WorkflowCallback: stepCallback,
		WorkflowEngine:   workflowEngine,
		TxPool:           txPool,
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
		return httpServer.Shutdown(shutdownCtx)
	})
}

// startWorker starts the job executor, worker pool, and scheduler goroutines.
func startWorker(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, q *queue.PostgresQueue, pub pubsub.Publisher, metrics *telemetry.Metrics, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine, healthReg *health.Registry) {
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
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                    p,
		Queue:                   q,
		Wake:                    wake,
		ConcurrencyLimit:        adaptive,
		Store:                   queries,
		PollInterval:            cfg.PollerInterval,
		HeartbeatInterval:       cfg.HeartbeatInterval,
		Publisher:               pub,
		Metrics:                 metrics,
		WorkflowCallback:        stepCallback,
		Partitions:              partitions,
		PartitionWeights:        partitionWeights,
		ExecutorHTTPTimeout:     cfg.ExecutorHTTPTimeout,
		ExecutorIdleConnTimeout: cfg.ExecutorIdleConnTimeout,
		WebhookTimeout:          cfg.WebhookTimeout,
		WebhookIdleConnTimeout:  cfg.WebhookIdleConnTimeout,
		WebhookDispatchTimeout:  cfg.WebhookDispatchTimeout,
		WebhookMaxAttempts:      cfg.WebhookMaxAttempts,
	})

	healthReg.Register(health.NewPoolChecker(p))

	queueDepthThreshold := int64(max(cfg.WorkerConcurrency*100, 1000))
	healthReg.Register(health.NewQueueDepthChecker(func(checkCtx context.Context) (int64, error) {
		stats, err := queries.QueueStats(checkCtx)
		if err != nil {
			return 0, err
		}
		return int64(stats.Queued), nil
	}, queueDepthThreshold))

	sched := scheduler.New(cfg, queries, q, stepCallback, workflowEngine, scheduler.WithSchedulerMetrics(metrics))
	schedulerMaxAge := max(cfg.PollerInterval*3, 30*time.Second)
	healthReg.Register(health.NewSchedulerChecker(sched.PollerLastTick, schedulerMaxAge))

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
	}

	g.Go(func(ctx context.Context) error {
		if adaptive != nil {
			adaptive.Run(ctx, 10*time.Second, func(probeCtx context.Context) (int, float64, error) {
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

	// Start scheduler (cron, delayed poller, reaper)
	g.Go(func(ctx context.Context) error {
		if err := sched.Start(ctx); err != nil {
			return fmt.Errorf("start scheduler: %w", err)
		}
		<-ctx.Done()
		slog.Info("shutting down scheduler")
		sched.Stop()
		return nil
	})
}

func runMigrations(databaseURL string) error {
	// Use pgx/v5/stdlib shim for database/sql compatibility
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open sql connection: %w", err)
	}
	defer db.Close()

	driver, err := pgmigrate.WithInstance(db, &pgmigrate.Config{})
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

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	slog.Info("migrations applied")
	return nil
}
