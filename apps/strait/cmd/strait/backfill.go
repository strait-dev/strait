package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/config"
	"strait/internal/store"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

func newBackfillHistoryCommand() *cobra.Command {
	var (
		batchSize int
		dryRun    bool
		timeout   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "backfill-history",
		Short: "Backfill terminal runs from job_runs into job_runs_history",
		Long:  "Moves terminal runs (completed, failed, timed_out, etc.) into the history table in batches. Safe to re-run.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if batchSize < 1 || batchSize > 10000 {
				return fmt.Errorf("--batch-size must be between 1 and 10000, got %d", batchSize)
			}
			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			return runBackfillHistory(ctx, batchSize, dryRun)
		},
	}

	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "rows per batch (1-10000)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "count rows without moving them")
	cmd.Flags().DurationVar(&timeout, "timeout", time.Hour, "maximum runtime duration (0 = unlimited)")

	return cmd
}

func runBackfillHistory(ctx context.Context, batchSize int, dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	if dryRun {
		count, err := queries.CountStrandedTerminalRuns(ctx, cfg.RunRetentionShort, cfg.RunRetentionLong)
		if err != nil {
			return fmt.Errorf("count stranded: %w", err)
		}
		slog.Info("dry run: stranded terminal runs", "count", count)
		return nil
	}

	cutoff := time.Now()
	var totalMoved int64

	for {
		moved, err := queries.BackfillTerminalRunsToHistory(ctx, cutoff, batchSize)
		if err != nil {
			return fmt.Errorf("backfill batch: %w", err)
		}
		totalMoved += moved
		if moved > 0 {
			slog.Info("backfill progress", "batch_moved", moved, "total_moved", totalMoved)
		}
		if moved < int64(batchSize) {
			break
		}
	}

	dupes, err := queries.CountDuplicateHistoryRuns(ctx)
	if err != nil {
		slog.Warn("failed to check duplicates", "error", err)
	} else if dupes > 0 {
		slog.Warn("duplicate rows found in both hot and history", "count", dupes)
	}

	slog.Info("backfill complete", "total_moved", totalMoved, "duplicates", dupes)
	return nil
}
