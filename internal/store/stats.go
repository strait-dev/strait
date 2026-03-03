package store

import (
	"context"
	"fmt"
)

type QueueStats struct {
	Queued    int `json:"queued"`
	Executing int `json:"executing"`
	Delayed   int `json:"delayed"`
}

func (q *Queries) QueueStats(ctx context.Context) (*QueueStats, error) {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END), 0) AS queued,
			COALESCE(SUM(CASE WHEN status = 'executing' THEN 1 ELSE 0 END), 0) AS executing,
			COALESCE(SUM(CASE WHEN status = 'delayed' THEN 1 ELSE 0 END), 0) AS delayed
		FROM job_runs
		WHERE status IN ('queued', 'executing', 'delayed')`

	var stats QueueStats
	err := q.db.QueryRow(ctx, query).Scan(&stats.Queued, &stats.Executing, &stats.Delayed)
	if err != nil {
		return nil, fmt.Errorf("queue stats: %w", err)
	}

	return &stats, nil
}
