package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/config"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// newBackfillEntitlementsCommand registers the cobra command that
// recomputes and persists the entitlements snapshot for every
// organization_subscriptions row. Idempotent: rows whose snapshot
// already matches ComputeEntitlements are skipped.
func newBackfillEntitlementsCommand() *cobra.Command {
	var (
		batchSize int
		dryRun    bool
		timeout   time.Duration
		orgID     string
	)

	cmd := &cobra.Command{
		Use:   "backfill-entitlements",
		Short: "Backfill the entitlements JSONB snapshot on organization_subscriptions",
		Long:  "Iterates organization_subscriptions, recomputes entitlements via billing.ComputeEntitlements, and writes any rows whose persisted snapshot is missing or stale. Safe to re-run.",
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
			return runBackfillEntitlements(ctx, batchSize, dryRun, orgID)
		},
	}

	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "rows per batch (1-10000)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report rows that would be updated without writing")
	cmd.Flags().DurationVar(&timeout, "timeout", time.Hour, "maximum runtime duration (0 = unlimited)")
	cmd.Flags().StringVar(&orgID, "org-id", "", "limit backfill to a single org (empty = all orgs)")

	return cmd
}

func runBackfillEntitlements(ctx context.Context, batchSize int, dryRun bool, singleOrgID string) error {
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

	pgStore := billing.NewPgStore(pool)
	progress := func(batchSize, batchUpdated int, totalScanned, totalUpdated int64) {
		slog.Info("entitlements backfill progress",
			"batch_size", batchSize,
			"batch_updated", batchUpdated,
			"total_scanned", totalScanned,
			"total_updated", totalUpdated,
			"dry_run", dryRun)
	}
	stats, err := billing.BackfillEntitlements(ctx, pool, pgStore, batchSize, dryRun, singleOrgID, progress)
	if err != nil {
		return err
	}

	slog.Info("entitlements backfill complete",
		"total_scanned", stats.Scanned, "total_updated", stats.Updated, "dry_run", dryRun)
	return nil
}
