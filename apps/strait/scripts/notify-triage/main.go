package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type failedMessage struct {
	ID                string
	ProjectID         string
	Channel           string
	SuppressionReason string
	CreatedAt         time.Time
}

type suppressionBucket struct {
	Reason string
	Count  int64
}

func main() {
	var (
		databaseURL    string
		projectID      string
		window         time.Duration
		staleThreshold time.Duration
		limit          int
	)

	flag.StringVar(&databaseURL, "database-url", envOr("DATABASE_URL", envOr("STRAIT_TEST_DATABASE_URL", "")), "PostgreSQL connection string")
	flag.StringVar(&projectID, "project-id", "", "Optional project_id filter")
	flag.DurationVar(&window, "window", 6*time.Hour, "Lookback window for recent failures")
	flag.DurationVar(&staleThreshold, "stale-threshold", 30*time.Minute, "Threshold used to flag stuck processing escalations")
	flag.IntVar(&limit, "limit", 20, "Max failed notification rows to print")
	flag.Parse()

	if strings.TrimSpace(databaseURL) == "" {
		exitf("DATABASE_URL (or -database-url) is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		exitf("connect database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		exitf("ping database: %v", err)
	}

	now := time.Now().UTC()
	fmt.Printf("notify triage snapshot\n")
	fmt.Printf("timestamp: %s\n", now.Format(time.RFC3339))
	if projectID != "" {
		fmt.Printf("project_id: %s\n", projectID)
	}
	fmt.Println()

	dueScheduled := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_messages
		 WHERE status = 'scheduled'
		   AND scheduled_at <= NOW()
		   AND ($1 = '' OR project_id = $1)`)

	processingMessages := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_messages
		 WHERE status = 'processing'
		   AND ($1 = '' OR project_id = $1)`)

	retryScheduled := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_messages
		 WHERE status = 'scheduled'
		   AND attempts > 0
		   AND scheduled_at > NOW()
		   AND ($1 = '' OR project_id = $1)`)

	overdueDigestBatches := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_batches
		 WHERE status = 'collecting'
		   AND window_end <= NOW()
		   AND ($1 = '' OR project_id = $1)`)

	processingDigestBatches := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_batches
		 WHERE status = 'processing'
		   AND ($1 = '' OR project_id = $1)`)

	failedDigestBatches := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM notification_batches
		 WHERE status = 'failed'
		   AND ($1 = '' OR project_id = $1)`)

	activeEscalationsDue := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM escalation_states
		 WHERE status = 'active'
		   AND next_escalation_at IS NOT NULL
		   AND next_escalation_at <= NOW()
		   AND ($1 = '' OR project_id = $1)`)

	stuckEscalations := mustCount(ctx, pool, projectID,
		`SELECT COUNT(*)
		 FROM escalation_states
		 WHERE status = 'processing'
		   AND updated_at <= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)`, staleThreshold.String())

	fmt.Println("queue health:")
	fmt.Printf("  due_scheduled_messages: %d\n", dueScheduled)
	fmt.Printf("  processing_messages:    %d\n", processingMessages)
	fmt.Printf("  retry_scheduled_msgs:   %d\n", retryScheduled)
	fmt.Printf("  overdue_digest_batches: %d\n", overdueDigestBatches)
	fmt.Printf("  processing_batches:     %d\n", processingDigestBatches)
	fmt.Printf("  failed_digest_batches:  %d\n", failedDigestBatches)
	fmt.Printf("  due_active_escalations: %d\n", activeEscalationsDue)
	fmt.Printf("  stuck_escalations:      %d\n", stuckEscalations)
	fmt.Println()

	suppressionTop := mustListSuppressionReasons(ctx, pool, projectID, window, 5)
	if len(suppressionTop) > 0 {
		fmt.Printf("top suppression reasons (window=%s):\n", window)
		for _, bucket := range suppressionTop {
			fmt.Printf("  - count=%d reason=%s\n", bucket.Count, sanitize(bucket.Reason))
		}
		fmt.Println()
	}

	failed := mustListFailedMessages(ctx, pool, projectID, window, limit)
	if len(failed) == 0 {
		fmt.Printf("recent failed notification messages: none (window=%s)\n", window)
		return
	}

	fmt.Printf("recent failed notification messages (window=%s):\n", window)
	for _, row := range failed {
		fmt.Printf("  - id=%s project=%s channel=%s at=%s reason=%s\n",
			row.ID,
			row.ProjectID,
			row.Channel,
			row.CreatedAt.Format(time.RFC3339),
			sanitize(row.SuppressionReason),
		)
	}
}

func mustCount(ctx context.Context, pool *pgxpool.Pool, projectID, query string, extraArgs ...any) int64 {
	args := []any{projectID}
	args = append(args, extraArgs...)
	var count int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		exitf("count query failed: %v", err)
	}
	return count
}

func mustListSuppressionReasons(ctx context.Context, pool *pgxpool.Pool, projectID string, window time.Duration, limit int) []suppressionBucket {
	if limit <= 0 {
		limit = 5
	}
	rows, err := pool.Query(ctx,
		`SELECT COALESCE(NULLIF(suppression_reason, ''), 'unknown') AS reason, COUNT(*)
		 FROM notification_messages
		 WHERE status = 'failed'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 GROUP BY reason
		 ORDER BY COUNT(*) DESC
		 LIMIT $3`,
		projectID,
		window.String(),
		limit,
	)
	if err != nil {
		exitf("suppression reason query failed: %v", err)
	}
	defer rows.Close()

	out := make([]suppressionBucket, 0, limit)
	for rows.Next() {
		var row suppressionBucket
		if err := rows.Scan(&row.Reason, &row.Count); err != nil {
			exitf("scan suppression reason row failed: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		exitf("iterate suppression reason rows failed: %v", err)
	}
	return out
}

func mustListFailedMessages(ctx context.Context, pool *pgxpool.Pool, projectID string, window time.Duration, limit int) []failedMessage {
	if limit <= 0 {
		limit = 20
	}
	rows, err := pool.Query(ctx,
		`SELECT id, project_id, channel, COALESCE(suppression_reason, ''), created_at
		 FROM notification_messages
		 WHERE status = 'failed'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 ORDER BY created_at DESC
		 LIMIT $3`,
		projectID,
		window.String(),
		limit,
	)
	if err != nil {
		exitf("list failed messages query failed: %v", err)
	}
	defer rows.Close()

	out := make([]failedMessage, 0, limit)
	for rows.Next() {
		var m failedMessage
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Channel, &m.SuppressionReason, &m.CreatedAt); err != nil {
			exitf("scan failed row: %v", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		exitf("iterate failed rows: %v", err)
	}
	return out
}

func sanitize(in string) string {
	in = strings.TrimSpace(strings.ReplaceAll(in, "\n", " "))
	if in == "" {
		return "n/a"
	}
	if len(in) > 140 {
		return in[:140] + "..."
	}
	return in
}

func envOr(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
