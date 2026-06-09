package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// Store helpers for query plan baselines used by the
// scheduler.PlanDriftMonitor.

// PlanBaselineRow mirrors the scheduler.PlanBaseline shape but lives in
// the store package to keep the scheduler independent of pgx types.
type PlanBaselineRow struct {
	QueryName    string
	TopNodeType  string
	EstTotalCost float64
	PlanJSON     []byte
}

// Explain runs EXPLAIN (FORMAT JSON) on the given SQL and returns the
// JSON bytes of the top-level plan. The SQL is executed against the
// same DB as the caller, so it must be a read-only query (the monitor
// only passes SELECTs).
func (q *Queries) Explain(ctx context.Context, sql string) ([]byte, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.Explain")
	defer span.End()

	trimmed := strings.TrimSpace(sql)
	if !isExplainableSelect(trimmed) {
		return nil, fmt.Errorf("explain: only single SELECT statements are allowed")
	}

	var out []byte
	err := q.db.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sql).Scan(&out)
	if err != nil {
		return nil, fmt.Errorf("explain: %w", err)
	}
	return out, nil
}

func isExplainableSelect(trimmedSQL string) bool {
	return trimmedSQL != "" &&
		strings.HasPrefix(strings.ToUpper(trimmedSQL), "SELECT") &&
		!strings.Contains(trimmedSQL, ";")
}

// GetPlanBaseline returns the stored baseline for `name`, if any.
// Second return value is false when no baseline exists yet.
func (q *Queries) GetPlanBaseline(ctx context.Context, name string) (PlanBaselineRow, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetPlanBaseline")
	defer span.End()

	var row PlanBaselineRow
	err := q.db.QueryRow(ctx, `
		SELECT query_name, top_node_type, est_total_cost, plan_json
		FROM query_plan_baselines
		WHERE query_name = $1
	`, name).Scan(&row.QueryName, &row.TopNodeType, &row.EstTotalCost, &row.PlanJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanBaselineRow{}, false, nil
		}
		return PlanBaselineRow{}, false, fmt.Errorf("get plan baseline: %w", err)
	}
	return row, true, nil
}

// UpsertPlanBaseline writes or replaces the stored baseline.
func (q *Queries) UpsertPlanBaseline(ctx context.Context, b PlanBaselineRow) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertPlanBaseline")
	defer span.End()

	const sql = `
		INSERT INTO query_plan_baselines (query_name, top_node_type, est_total_cost, plan_json, captured_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (query_name)
		DO UPDATE SET top_node_type = EXCLUDED.top_node_type,
		              est_total_cost = EXCLUDED.est_total_cost,
		              plan_json = EXCLUDED.plan_json,
		              captured_at = NOW()`
	if _, err := q.db.Exec(ctx, sql, b.QueryName, b.TopNodeType, b.EstTotalCost, b.PlanJSON); err != nil {
		return fmt.Errorf("upsert plan baseline: %w", err)
	}
	return nil
}
