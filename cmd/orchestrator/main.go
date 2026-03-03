package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orchestrator/internal/api"
	"orchestrator/internal/config"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/scheduler"
	"orchestrator/internal/store"
	"orchestrator/internal/telemetry"
	"orchestrator/internal/worker"
	"orchestrator/migrations"

	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse flags
	mode := flag.String("mode", "", "run mode: api, worker, or all (overrides MODE env)")
	flag.Parse()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// CLI flag overrides env
	if *mode != "" {
		cfg.Mode = *mode
	}

	// Validate mode
	switch cfg.Mode {
	case "api", "worker", "all":
	default:
		return fmt.Errorf("invalid mode %q: must be api, worker, or all", cfg.Mode)
	}

	// Set up structured logging
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	slog.Info("starting orchestrator",
		"version", version,
		"mode", cfg.Mode,
		"port", cfg.Port,
	)

	// Context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Initialize OpenTelemetry tracing
	shutdownTracer, err := telemetry.Init(ctx, "orchestrator", cfg.OTELEndpoint)
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

	// Connect to Postgres
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	slog.Info("connected to postgres")

	// Run migrations
	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Create dependencies
	queries := store.New(pool)
	q := queue.NewPostgresQueue(pool)

	// Connect to Redis for pubsub
	var pub pubsub.Publisher
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("parse redis url: %w", err)
		}
		rdb := redis.NewClient(opts)
		if err := rdb.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("ping redis: %w", err)
		}
		slog.Info("connected to redis")
		pub = pubsub.NewRedisPublisher(rdb)
		defer rdb.Close()
	}

	// Error group for concurrent goroutines
	g, gCtx := errgroup.WithContext(ctx)

	// Start API server
	if cfg.Mode == "api" || cfg.Mode == "all" {
		srv := api.NewServer(cfg, queries, q, pub)
		httpServer := &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Port),
			Handler:           srv,
			ReadHeaderTimeout: 10 * time.Second,
		}

		g.Go(func() error {
			slog.Info("api server listening", "addr", httpServer.Addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http server: %w", err)
			}
			return nil
		})

		g.Go(func() error {
			<-gCtx.Done()
			slog.Info("shutting down api server")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			return httpServer.Shutdown(shutdownCtx)
		})
	}

	// Start worker executor
	if cfg.Mode == "worker" || cfg.Mode == "all" {
		p := worker.NewPool(cfg.WorkerConcurrency)
		exec := worker.NewExecutor(worker.ExecutorConfig{
			Pool:              p,
			Queue:             q,
			Store:             queries,
			PollInterval:      cfg.PollerInterval,
			HeartbeatInterval: cfg.HeartbeatInterval,
			Publisher:         pub,
		})

		g.Go(func() error {
			exec.Run(gCtx)
			return nil
		})

		g.Go(func() error {
			<-gCtx.Done()
			slog.Info("shutting down worker pool")
			p.Shutdown()
			return nil
		})

		// Start scheduler (cron, delayed poller, reaper)
		sched := scheduler.New(cfg, queries, q)
		if err := sched.Start(gCtx); err != nil {
			return fmt.Errorf("start scheduler: %w", err)
		}

		g.Go(func() error {
			<-gCtx.Done()
			slog.Info("shutting down scheduler")
			sched.Stop()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("errgroup: %w", err)
	}

	slog.Info("orchestrator stopped")
	return nil
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
