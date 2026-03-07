package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateWorkflowRunLabels")
	defer span.End()

	if len(labels) == 0 {
		return nil
	}

	for k, v := range labels {
		if _, err := q.db.Exec(ctx,
			`INSERT INTO workflow_run_labels (workflow_run_id, label_key, label_value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (workflow_run_id, label_key) DO UPDATE SET label_value = EXCLUDED.label_value`,
			workflowRunID, k, v,
		); err != nil {
			return fmt.Errorf("insert workflow run label %s: %w", k, err)
		}
	}

	return nil
}

func (q *Queries) ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListWorkflowRunLabels")
	defer span.End()

	rows, err := q.db.Query(ctx,
		`SELECT label_key, label_value FROM workflow_run_labels WHERE workflow_run_id = $1 ORDER BY label_key ASC`,
		workflowRunID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflow run labels: %w", err)
	}
	defer rows.Close()

	labels := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan workflow run labels: %w", err)
		}
		labels[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow run labels rows: %w", err)
	}

	return labels, nil
}
