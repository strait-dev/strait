package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

// EventTriggerStats holds aggregate statistics about event triggers.
type EventTriggerStats struct {
	TotalCount      int     `json:"total_count"`
	WaitingCount    int     `json:"waiting_count"`
	ReceivedCount   int     `json:"received_count"`
	TimedOutCount   int     `json:"timed_out_count"`
	CanceledCount   int     `json:"canceled_count"`
	AvgWaitDuration float64 `json:"avg_wait_duration_secs"` // average seconds from requested_at to received_at
}

// GetEventTriggerStats returns aggregate statistics for a project's event triggers.
// When environmentID is non-empty, only triggers in that environment are included.
func (q *Queries) GetEventTriggerStats(ctx context.Context, projectID, environmentID string) (*EventTriggerStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventTriggerStats")
	defer span.End()

	var stats EventTriggerStats

	countQuery := `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'waiting') AS waiting,
			COUNT(*) FILTER (WHERE status = 'received') AS received,
			COUNT(*) FILTER (WHERE status = 'timed_out') AS timed_out,
			COUNT(*) FILTER (WHERE status = 'canceled') AS canceled
			FROM event_triggers
			WHERE project_id = $1
			  AND ($2 = '' OR environment_id = $2)`

	err := q.db.QueryRow(ctx, countQuery, projectID, environmentID).Scan(
		&stats.TotalCount,
		&stats.WaitingCount,
		&stats.ReceivedCount,
		&stats.TimedOutCount,
		&stats.CanceledCount,
	)
	if err != nil {
		return nil, fmt.Errorf("count event triggers: %w", err)
	}

	avgQuery := `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (received_at - requested_at))), 0)
		FROM event_triggers
		WHERE project_id = $1
		  AND ($2 = '' OR environment_id = $2)
		  AND status = 'received'
		  AND received_at IS NOT NULL`

	err = q.db.QueryRow(ctx, avgQuery, projectID, environmentID).Scan(&stats.AvgWaitDuration)
	if err != nil {
		return nil, fmt.Errorf("avg wait duration: %w", err)
	}

	return &stats, nil
}
