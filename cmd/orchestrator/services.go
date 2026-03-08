package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"orchestrator/internal/api"
	"orchestrator/internal/cdc"
	"orchestrator/internal/config"
	"orchestrator/internal/health"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/scheduler"
	"orchestrator/internal/store"
	"orchestrator/internal/telemetry"
	"orchestrator/internal/worker"
	"orchestrator/internal/workflow"
	"orchestrator/migrations"

	"github.com/exaring/otelpgx"
	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sourcegraph/conc/pool"
)

// connectDatabase creates and verifies a Postgres connection pool.
// It retries with exponential backoff up to 5 times on transient failures.
func connectDatabase(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
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
func connectRedis(ctx context.Context, cfg *config.Config) (pubsub.Publisher, interface{ Close() error }, error) {
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

	g.Go(func(ctx context.Context) error {
		cdcConsumer.Run(ctx)
		return nil
	})

	slog.Info("cdc consumer enabled",
		"base_url", cfg.SequinBaseURL,
		"consumer", cfg.SequinConsumerName,
	)
}

// startAPIServer starts the HTTP API server and its graceful shutdown goroutine.
func startAPIServer(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, q *queue.PostgresQueue, pub pubsub.Publisher, metricsHandler http.Handler, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine) {
	if cfg.Mode != "api" && cfg.Mode != "all" {
		return
	}

	var pinger api.Pinger
	if redisPub, ok := pub.(*pubsub.RedisPublisher); ok {
		pinger = redisPub
	}

	healthReg := health.NewRegistry()
	healthReg.Register(health.NewChecker("database", func(ctx context.Context) error {
		_, err := queries.QueueStats(ctx)
		return err
	}))
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
		Pinger:           pinger,
		HealthRegistry:   healthReg,
		WorkflowCallback: stepCallback,
		WorkflowEngine:   workflowEngine,
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
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
}

// startWorker starts the job executor, worker pool, and scheduler goroutines.
func startWorker(g *pool.ContextPool, cfg *config.Config, queries *store.Queries, q *queue.PostgresQueue, pub pubsub.Publisher, metrics *telemetry.Metrics, stepCallback *workflow.StepCallback, workflowEngine *workflow.WorkflowEngine) {
	if cfg.Mode != "worker" && cfg.Mode != "all" {
		return
	}

	p := worker.NewPool(cfg.WorkerConcurrency)
	partitions := []string(nil)
	partitionWeights := ""
	if cfg.FFQueuePartitioning && len(cfg.WorkerPartitions) > 0 {
		partitions = cfg.WorkerPartitions
		partitionWeights = cfg.WorkerPartitionWeights
		slog.Info("worker queue partitioning enabled", "partitions", partitions)
	}
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                    p,
		Queue:                   q,
		Store:                   queries,
		PollInterval:            cfg.PollerInterval,
		HeartbeatInterval:       cfg.HeartbeatInterval,
		Publisher:               pub,
		Metrics:                 metrics,
		WorkflowCallback:        stepCallback,
		Partitions:              partitions,
		PartitionWeights:        partitionWeights,
		CircuitBreaker:          cfg.FFCircuitBreaker,
		SmartRetry:              cfg.FFSmartRetry,
		Bulkheads:               cfg.FFBulkheads,
		SecretInjection:         cfg.FFSecretInjection,
		ExecutionTracing:        cfg.FFExecutionTracing,
		AdaptiveTimeout:         cfg.FFAdaptiveTimeout,
		DLQEnabled:              cfg.FFRunDLQ,
		ExecutorHTTPTimeout:     cfg.ExecutorHTTPTimeout,
		ExecutorIdleConnTimeout: cfg.ExecutorIdleConnTimeout,
		WebhookTimeout:          cfg.WebhookTimeout,
		WebhookIdleConnTimeout:  cfg.WebhookIdleConnTimeout,
		WebhookDispatchTimeout:  cfg.WebhookDispatchTimeout,
		WebhookMaxAttempts:      cfg.WebhookMaxAttempts,
	})

	g.Go(func(ctx context.Context) error {
		exec.Run(ctx)
		return nil
	})

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		slog.Info("shutting down worker pool")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := p.Shutdown(shutdownCtx); err != nil {
			slog.Warn("worker pool shutdown timed out", "error", err)
		}
		return nil
	})

	// Start scheduler (cron, delayed poller, reaper)
	sched := scheduler.New(cfg, queries, q, stepCallback, workflowEngine)
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
