package store

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRunLabels")
	defer span.End()

	if len(labels) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO workflow_run_labels (workflow_run_id, label_key, label_value) VALUES `)
	args := make([]any, 0, len(labels)*3)
	i := 0
	for k, v := range labels {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "($%d, $%d, $%d)", i*3+1, i*3+2, i*3+3)
		args = append(args, workflowRunID, k, v)
		i++
	}
	sb.WriteString(` ON CONFLICT (workflow_run_id, label_key) DO UPDATE SET label_value = EXCLUDED.label_value
		WHERE workflow_run_labels.label_value IS DISTINCT FROM EXCLUDED.label_value`)

	if _, err := q.db.Exec(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("batch insert workflow run labels: %w", err)
	}

	return nil
}

func (q *Queries) ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRunLabels")
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
