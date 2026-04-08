package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
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

type promQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type checkOptions struct {
	queueURL                   string
	region                     string
	databaseURL                string
	projectID                  string
	window                     time.Duration
	failOnBacklog              int64
	minCallbacks               int64
	prometheusURL              string
	prometheusTimeout          time.Duration
	promQueryWindow            time.Duration
	maxCallbackErrorRatePct    float64
	maxCallbackDuplicatePct    float64
	maxBounceComplaintRatioPct float64
}

func main() {
	opts := parseOptions()
	if err := validateOptions(opts); err != nil {
		exitf("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	snapshot, err := fetchQueueSnapshot(ctx, opts.queueURL, opts.region)
	if err != nil {
		exitf("fetch queue snapshot: %v", err)
	}

	printQueueSnapshot(snapshot)
	if opts.failOnBacklog >= 0 && snapshot.Visible > opts.failOnBacklog {
		exitf("visible backlog %d is above threshold %d", snapshot.Visible, opts.failOnBacklog)
	}

	if opts.databaseURL == "" {
		fmt.Println("database snapshot: skipped (database-url not set)")
	} else if err := runDatabaseSnapshot(ctx, opts); err != nil {
		exitf("database snapshot failed: %v", err)
	}

	if opts.prometheusURL != "" {
		if err := runPrometheusChecks(opts); err != nil {
			exitf("prometheus checks failed: %v", err)
		}
	}

	fmt.Println("result: ok")
}

func parseOptions() checkOptions {
	opts := checkOptions{}
	flag.StringVar(&opts.queueURL, "queue-url", envOr("NOTIFY_SES_FEEDBACK_SQS_URL", ""), "SES feedback SQS queue URL")
	flag.StringVar(&opts.region, "region", envOr("SES_REGION", ""), "AWS region for the feedback queue (optional; inferred from queue URL when empty)")
	flag.StringVar(&opts.databaseURL, "database-url", envOr("DATABASE_URL", envOr("STRAIT_TEST_DATABASE_URL", "")), "Optional PostgreSQL DSN for callback/suppression snapshot")
	flag.StringVar(&opts.projectID, "project-id", "", "Optional project_id filter for DB snapshot")
	flag.DurationVar(&opts.window, "window", 6*time.Hour, "Lookback window for DB snapshot")
	flag.Int64Var(&opts.failOnBacklog, "fail-on-backlog", -1, "Fail if visible queue backlog is greater than this threshold (-1 disables)")
	flag.Int64Var(&opts.minCallbacks, "min-callbacks", 0, "Fail if SES callback receipts in window are below this threshold (requires database-url)")
	flag.StringVar(&opts.prometheusURL, "prometheus-url", envOr("PROMETHEUS_URL", ""), "Optional Prometheus base URL used for callback outcome ratio checks")
	flag.DurationVar(&opts.prometheusTimeout, "prometheus-timeout", 10*time.Second, "Timeout for Prometheus API requests")
	flag.DurationVar(&opts.promQueryWindow, "prom-window", 5*time.Minute, "Range window used in PromQL rate() checks")
	flag.Float64Var(&opts.maxCallbackErrorRatePct, "max-callback-error-rate", -1, "Fail if SES callback processing error percentage exceeds this value (-1 disables)")
	flag.Float64Var(&opts.maxCallbackDuplicatePct, "max-callback-duplicate-ratio", -1, "Fail if SES duplicate callback percentage exceeds this value (-1 disables)")
	flag.Float64Var(&opts.maxBounceComplaintRatioPct, "max-bounce-complaint-ratio", -1, "Fail if SES bounce/complaint ratio percentage exceeds this value (-1 disables)")
	flag.Parse()

	if strings.TrimSpace(opts.region) == "" {
		opts.region = inferRegionFromQueueURL(opts.queueURL)
	}
	return opts
}

func validateOptions(opts checkOptions) error {
	if strings.TrimSpace(opts.queueURL) == "" {
		return errors.New("queue-url is required (or set NOTIFY_SES_FEEDBACK_SQS_URL)")
	}
	if strings.TrimSpace(opts.prometheusURL) == "" && hasPrometheusThreshold(opts) {
		return errors.New("prometheus-url is required when callback ratio thresholds are enabled")
	}
	return nil
}

func runDatabaseSnapshot(ctx context.Context, opts checkOptions) error {
	pool, err := pgxpool.New(ctx, opts.databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if opts.projectID != "" {
		fmt.Printf("project_id filter: %s\n\n", opts.projectID)
	}

	callbacks := mustListBuckets(
		ctx,
		pool,
		opts.projectID,
		opts.window,
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

	printBuckets(fmt.Sprintf("ses callback receipts (window=%s):", opts.window), "event_type", callbacks)

	suppressions := mustListBuckets(
		ctx,
		pool,
		opts.projectID,
		opts.window,
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
	printBuckets(fmt.Sprintf("ses suppression events (window=%s):", opts.window), "reason", suppressions)

	statuses := mustListBuckets(
		ctx,
		pool,
		opts.projectID,
		opts.window,
		`SELECT status AS bucket_key, COUNT(*)
		 FROM notification_messages
		 WHERE channel = 'email'
		   AND created_at >= NOW() - $2::interval
		   AND ($1 = '' OR project_id = $1)
		 GROUP BY status
		 ORDER BY COUNT(*) DESC`,
	)
	printBuckets(fmt.Sprintf("email message statuses (window=%s):", opts.window), "status", statuses)

	if opts.minCallbacks > 0 && callbackTotal < opts.minCallbacks {
		return fmt.Errorf("ses callback receipts %d below min-callbacks threshold %d", callbackTotal, opts.minCallbacks)
	}
	return nil
}

func printQueueSnapshot(snapshot *queueSnapshot) {
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
}

func printBuckets(header, keyLabel string, buckets []callbackBucket) {
	fmt.Println(header)
	if len(buckets) == 0 {
		fmt.Println("  - none")
		fmt.Println()
		return
	}
	for _, bucket := range buckets {
		fmt.Printf("  - count=%d %s=%s\n", bucket.Count, keyLabel, sanitize(bucket.Key))
	}
	fmt.Println()
}

func runPrometheusChecks(opts checkOptions) error {
	rangeWindow := promRangeLiteral(opts.promQueryWindow)
	errorRateExpr := fmt.Sprintf(`100 * sum(rate(strait_notify_provider_callbacks_total{provider="ses",outcome=~".*_error|malformed"}[%s])) / clamp_min(sum(rate(strait_notify_provider_callbacks_total{provider="ses"}[%s])), 0.0001)`, rangeWindow, rangeWindow)
	duplicateRatioExpr := fmt.Sprintf(`100 * sum(rate(strait_notify_provider_callbacks_total{provider="ses",outcome="duplicate"}[%s])) / clamp_min(sum(rate(strait_notify_provider_callbacks_total{provider="ses"}[%s])), 0.0001)`, rangeWindow, rangeWindow)
	bounceComplaintExpr := fmt.Sprintf(`100 * sum(rate(strait_notify_provider_callbacks_total{provider="ses",suppression_reason=~"provider_callback:ses.bounce|provider_callback:ses.complaint",outcome=~"status_updated|status_skipped"}[%s])) / clamp_min(sum(rate(strait_notify_provider_callbacks_total{provider="ses",outcome=~"status_updated|status_skipped"}[%s])), 0.0001)`, rangeWindow, rangeWindow)

	promCtx, promCancel := context.WithTimeout(context.Background(), opts.prometheusTimeout)
	defer promCancel()

	errorRatePct, err := queryPrometheusScalar(promCtx, opts.prometheusURL, errorRateExpr)
	if err != nil {
		return fmt.Errorf("query callback error rate: %w", err)
	}
	duplicateRatioPct, err := queryPrometheusScalar(promCtx, opts.prometheusURL, duplicateRatioExpr)
	if err != nil {
		return fmt.Errorf("query duplicate ratio: %w", err)
	}
	bounceComplaintRatioPct, err := queryPrometheusScalar(promCtx, opts.prometheusURL, bounceComplaintExpr)
	if err != nil {
		return fmt.Errorf("query bounce/complaint ratio: %w", err)
	}

	fmt.Println("prometheus callback ratios:")
	fmt.Printf("  callback_error_rate_pct:       %.4f\n", errorRatePct)
	fmt.Printf("  callback_duplicate_ratio_pct:  %.4f\n", duplicateRatioPct)
	fmt.Printf("  bounce_complaint_ratio_pct:    %.4f\n", bounceComplaintRatioPct)
	fmt.Println()

	if opts.maxCallbackErrorRatePct >= 0 && errorRatePct > opts.maxCallbackErrorRatePct {
		return fmt.Errorf("callback error rate %.4f exceeds threshold %.4f", errorRatePct, opts.maxCallbackErrorRatePct)
	}
	if opts.maxCallbackDuplicatePct >= 0 && duplicateRatioPct > opts.maxCallbackDuplicatePct {
		return fmt.Errorf("callback duplicate ratio %.4f exceeds threshold %.4f", duplicateRatioPct, opts.maxCallbackDuplicatePct)
	}
	if opts.maxBounceComplaintRatioPct >= 0 && bounceComplaintRatioPct > opts.maxBounceComplaintRatioPct {
		return fmt.Errorf("bounce/complaint ratio %.4f exceeds threshold %.4f", bounceComplaintRatioPct, opts.maxBounceComplaintRatioPct)
	}
	return nil
}

func hasPrometheusThreshold(opts checkOptions) bool {
	return opts.maxCallbackErrorRatePct >= 0 || opts.maxCallbackDuplicatePct >= 0 || opts.maxBounceComplaintRatioPct >= 0
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

func queryPrometheusScalar(ctx context.Context, prometheusURL, query string) (float64, error) {
	u, err := url.Parse(strings.TrimSpace(prometheusURL))
	if err != nil {
		return 0, fmt.Errorf("parse prometheus url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/query"
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("build prometheus request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute prometheus request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("prometheus request failed with status %d", resp.StatusCode)
	}

	parsed := promQueryResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decode prometheus response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query status=%s error=%s", parsed.Status, parsed.Error)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, nil
	}
	if len(parsed.Data.Result[0].Value) < 2 {
		return 0, nil
	}

	rawValue := fmt.Sprintf("%v", parsed.Data.Result[0].Value[1])
	value, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
	if err != nil {
		return 0, fmt.Errorf("parse prometheus scalar value %q: %w", rawValue, err)
	}
	return value, nil
}

func promRangeLiteral(d time.Duration) string {
	if d <= 0 {
		d = 5 * time.Minute
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
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
