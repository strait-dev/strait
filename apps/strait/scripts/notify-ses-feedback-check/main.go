package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv2config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queueSnapshot struct {
	QueueURL string
	QueueARN string
	Visible  int64
	InFlight int64
	Delayed  int64
}

type callbackBucket struct {
	Key   string
	Count int64
}

func main() {
	var (
		queueURL      string
		region        string
		databaseURL   string
		projectID     string
		window        time.Duration
		failOnBacklog int64
		minCallbacks  int64
	)

	flag.StringVar(&queueURL, "queue-url", envOr("NOTIFY_SES_FEEDBACK_SQS_URL", ""), "SES feedback SQS queue URL")
	flag.StringVar(&region, "region", envOr("SES_REGION", ""), "AWS region for the feedback queue (optional; inferred from queue URL when empty)")
	flag.StringVar(&databaseURL, "database-url", envOr("DATABASE_URL", envOr("STRAIT_TEST_DATABASE_URL", "")), "Optional PostgreSQL DSN for callback/suppression snapshot")
	flag.StringVar(&projectID, "project-id", "", "Optional project_id filter for DB snapshot")
	flag.DurationVar(&window, "window", 6*time.Hour, "Lookback window for DB snapshot")
	flag.Int64Var(&failOnBacklog, "fail-on-backlog", -1, "Fail if visible queue backlog is greater than this threshold (-1 disables)")
	flag.Int64Var(&minCallbacks, "min-callbacks", 0, "Fail if SES callback receipts in window are below this threshold (requires database-url)")
	flag.Parse()

	if strings.TrimSpace(queueURL) == "" {
		exitf("queue-url is required (or set NOTIFY_SES_FEEDBACK_SQS_URL)")
	}
	if strings.TrimSpace(region) == "" {
		region = inferRegionFromQueueURL(queueURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	snapshot, err := fetchQueueSnapshot(ctx, queueURL, region)
	if err != nil {
		exitf("fetch queue snapshot: %v", err)
	}

	now := time.Now().UTC()
	fmt.Printf("notify ses feedback check\n")
	fmt.Printf("timestamp: %s\n", now.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("sqs queue snapshot:")
	fmt.Printf("  queue_url:          %s\n", snapshot.QueueURL)
	fmt.Printf("  queue_arn:          %s\n", snapshot.QueueARN)
	fmt.Printf("  visible:            %d\n", snapshot.Visible)
	fmt.Printf("  in_flight:          %d\n", snapshot.InFlight)
	fmt.Printf("  delayed:            %d\n", snapshot.Delayed)
	fmt.Println()

	if failOnBacklog >= 0 && snapshot.Visible > failOnBacklog {
		exitf("visible backlog %d is above threshold %d", snapshot.Visible, failOnBacklog)
	}

	if strings.TrimSpace(databaseURL) == "" {
		fmt.Println("database snapshot: skipped (database-url not set)")
		fmt.Println("result: ok")
		return
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		exitf("connect database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		exitf("ping database: %v", err)
	}

	if projectID != "" {
		fmt.Printf("project_id filter: %s\n\n", projectID)
	}

	callbacks := mustListBuckets(
		ctx,
		pool,
		projectID,
		window,
		`SELECT COALESCE(NULLIF(event_type, ''), 'unknown') AS bucket_key, COUNT(*)
		 FROM notify_provider_callback_receipts
		 WHERE provider = 'ses'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 GROUP BY bucket_key
		 ORDER BY COUNT(*) DESC`,
	)
	callbackTotal := int64(0)
	for _, b := range callbacks {
		callbackTotal += b.Count
	}

	fmt.Printf("ses callback receipts (window=%s):\n", window)
	if len(callbacks) == 0 {
		fmt.Println("  - none")
	} else {
		for _, bucket := range callbacks {
			fmt.Printf("  - count=%d event_type=%s\n", bucket.Count, sanitize(bucket.Key))
		}
	}
	fmt.Println()

	suppressions := mustListBuckets(
		ctx,
		pool,
		projectID,
		window,
		`SELECT COALESCE(NULLIF(reason, ''), 'unknown') AS bucket_key, COUNT(*)
		 FROM notify_suppression_events
		 WHERE source = 'provider_callback'
		   AND channel = 'email'
		   AND action = 'suppressed'
		   AND reason LIKE 'provider_callback:ses.%'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 GROUP BY bucket_key
		 ORDER BY COUNT(*) DESC`,
	)

	fmt.Printf("ses suppression events (window=%s):\n", window)
	if len(suppressions) == 0 {
		fmt.Println("  - none")
	} else {
		for _, bucket := range suppressions {
			fmt.Printf("  - count=%d reason=%s\n", bucket.Count, sanitize(bucket.Key))
		}
	}
	fmt.Println()

	statuses := mustListBuckets(
		ctx,
		pool,
		projectID,
		window,
		`SELECT status AS bucket_key, COUNT(*)
		 FROM notification_messages
		 WHERE channel = 'email'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 GROUP BY status
		 ORDER BY COUNT(*) DESC`,
	)

	fmt.Printf("email message statuses (window=%s):\n", window)
	if len(statuses) == 0 {
		fmt.Println("  - none")
	} else {
		for _, bucket := range statuses {
			fmt.Printf("  - count=%d status=%s\n", bucket.Count, sanitize(bucket.Key))
		}
	}
	fmt.Println()

	if minCallbacks > 0 && callbackTotal < minCallbacks {
		exitf("ses callback receipts %d below min-callbacks threshold %d", callbackTotal, minCallbacks)
	}

	fmt.Println("result: ok")
}

func fetchQueueSnapshot(ctx context.Context, queueURL, region string) (*queueSnapshot, error) {
	loadOptions := []func(*awsv2config.LoadOptions) error{}
	if strings.TrimSpace(region) != "" {
		loadOptions = append(loadOptions, awsv2config.WithRegion(strings.TrimSpace(region)))
	}

	accessKey := strings.TrimSpace(os.Getenv("SES_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("SES_SECRET_ACCESS_KEY"))
	sessionToken := strings.TrimSpace(os.Getenv("SES_SESSION_TOKEN"))
	if accessKey != "" || secretKey != "" {
		loadOptions = append(loadOptions, awsv2config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey,
			secretKey,
			sessionToken,
		)))
	}

	awsCfg, err := awsv2config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := sqs.NewFromConfig(awsCfg)
	resp, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: awsv2.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameQueueArn,
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get queue attributes: %w", err)
	}

	attrs := resp.Attributes
	return &queueSnapshot{
		QueueURL: queueURL,
		QueueARN: attrs[string(sqstypes.QueueAttributeNameQueueArn)],
		Visible:  parseAttributeInt(attrs[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)]),
		InFlight: parseAttributeInt(attrs[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)]),
		Delayed:  parseAttributeInt(attrs[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed)]),
	}, nil
}

func mustListBuckets(ctx context.Context, pool *pgxpool.Pool, projectID string, window time.Duration, query string) []callbackBucket {
	rows, err := pool.Query(ctx, query, projectID, window.String())
	if err != nil {
		exitf("bucket query failed: %v", err)
	}
	defer rows.Close()

	out := make([]callbackBucket, 0, 8)
	for rows.Next() {
		var bucket callbackBucket
		if err := rows.Scan(&bucket.Key, &bucket.Count); err != nil {
			exitf("scan bucket row failed: %v", err)
		}
		out = append(out, bucket)
	}
	if err := rows.Err(); err != nil {
		exitf("iterate bucket rows failed: %v", err)
	}
	return out
}

func parseAttributeInt(raw string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func inferRegionFromQueueURL(queueURL string) string {
	u, err := url.Parse(strings.TrimSpace(queueURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if !strings.HasPrefix(host, "sqs.") || !strings.HasSuffix(host, ".amazonaws.com") {
		return ""
	}
	trimmed := strings.TrimPrefix(host, "sqs.")
	trimmed = strings.TrimSuffix(trimmed, ".amazonaws.com")
	return strings.TrimSpace(trimmed)
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
