package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackfillEntitlementsStats summarizes a single backfill invocation.
type BackfillEntitlementsStats struct {
	Scanned int64
	Updated int64
}

// BackfillEntitlementsProgress is invoked once per processed batch so callers
// can emit progress logs without owning the iteration loop.
type BackfillEntitlementsProgress func(batchSize, batchUpdated int, totalScanned, totalUpdated int64)

// BackfillEntitlements iterates organization_subscriptions in keyset order,
// recomputes the entitlements snapshot via ComputeEntitlements, and writes
// rows whose persisted snapshot is missing or stale. Idempotent: a second
// call is a no-op when nothing has changed.
//
// Pass an empty singleOrgID to backfill all rows; pass a specific orgID to
// scope to a single subscription. batchSize controls keyset page size when
// scanning all rows.
func BackfillEntitlements(
	ctx context.Context,
	pool *pgxpool.Pool,
	store *PgStore,
	batchSize int,
	dryRun bool,
	singleOrgID string,
	progress BackfillEntitlementsProgress,
) (BackfillEntitlementsStats, error) {
	var stats BackfillEntitlementsStats

	if singleOrgID != "" {
		updated, err := backfillOneOrgRow(ctx, pool, store, singleOrgID, dryRun)
		if err != nil {
			return stats, fmt.Errorf("backfill org %s: %w", singleOrgID, err)
		}
		stats.Scanned = 1
		if updated {
			stats.Updated = 1
		}
		if progress != nil {
			progress(1, int(stats.Updated), stats.Scanned, stats.Updated)
		}
		return stats, nil
	}

	var cursor string
	for {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		orgIDs, err := nextEntitlementsBatch(ctx, pool, cursor, batchSize)
		if err != nil {
			return stats, fmt.Errorf("fetch batch: %w", err)
		}
		if len(orgIDs) == 0 {
			break
		}

		var batchUpdated int
		for _, id := range orgIDs {
			updated, err := backfillOneOrgRow(ctx, pool, store, id, dryRun)
			if err != nil {
				return stats, fmt.Errorf("backfill org %s: %w", id, err)
			}
			if updated {
				batchUpdated++
			}
		}
		stats.Scanned += int64(len(orgIDs))
		stats.Updated += int64(batchUpdated)
		if progress != nil {
			progress(len(orgIDs), batchUpdated, stats.Scanned, stats.Updated)
		}

		cursor = orgIDs[len(orgIDs)-1]
		if len(orgIDs) < batchSize {
			break
		}
	}
	return stats, nil
}

func nextEntitlementsBatch(ctx context.Context, pool *pgxpool.Pool, cursor string, batchSize int) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT org_id FROM organization_subscriptions
		WHERE org_id > $1
		ORDER BY org_id
		LIMIT $2
	`, cursor, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func backfillOneOrgRow(ctx context.Context, pool *pgxpool.Pool, store *PgStore, orgID string, dryRun bool) (bool, error) {
	sub, err := store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("get sub: %w", err)
	}
	addons, err := store.ListActiveAddons(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("list addons: %w", err)
	}

	want := ComputeEntitlements(sub, addons)
	current, err := readPersistedEntitlements(ctx, pool, orgID)
	if err != nil {
		return false, fmt.Errorf("read persisted: %w", err)
	}
	if entitlementsEqual(current, want) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	updated, err := UpdateEntitlementsIfUnchanged(ctx, pool, orgID, want, sub.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	return updated, nil
}

// UpdateEntitlementsIfUnchanged writes entitlements only if the subscription
// row has not changed since observedUpdatedAt. Backfill uses this to avoid
// overwriting fresher webhook or operator mutations with a stale snapshot.
func UpdateEntitlementsIfUnchanged(ctx context.Context, pool *pgxpool.Pool, orgID string, entitlements OrgPlanLimits, observedUpdatedAt time.Time) (bool, error) {
	payload, err := json.Marshal(entitlements)
	if err != nil {
		return false, fmt.Errorf("marshal entitlements: %w", err)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET entitlements = $2::jsonb, updated_at = NOW()
		WHERE org_id = $1
		  AND updated_at = $3
	`, orgID, payload, observedUpdatedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func readPersistedEntitlements(ctx context.Context, pool *pgxpool.Pool, orgID string) (OrgPlanLimits, error) {
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrgPlanLimits{}, nil
		}
		return OrgPlanLimits{}, err
	}
	if len(raw) == 0 {
		return OrgPlanLimits{}, nil
	}
	var got OrgPlanLimits
	if err := json.Unmarshal(raw, &got); err != nil {
		// Treat undecodable column as drift so the backfill rewrites it.
		return OrgPlanLimits{}, nil //nolint:nilerr // corrupt persisted JSON is drift; backfill should rewrite it
	}
	return got, nil
}

func entitlementsEqual(a, b OrgPlanLimits) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}
