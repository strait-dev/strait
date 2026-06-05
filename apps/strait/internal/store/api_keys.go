package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ErrAPIKeyNotFound is returned by lookups (GetAPIKeyByHash, GetAPIKeyByID)
// when the row does not exist or has been hard-deleted. Callers MUST match
// it via errors.Is — string comparison on err.Error() drifts the moment a
// future caller wraps the error and there is no compile-time signal.
var ErrAPIKeyNotFound = errors.New("api key not found")

// apiKeyTouchCooldown is the window during which repeated TouchAPIKeyLastUsed
// calls for the same key are coalesced into a single UPDATE. A hot key making
// thousands of requests per minute previously generated one UPDATE per
// request — wasted WAL, contention on the row, and noise in replication.
// 60s is the smallest window that still keeps "last seen" useful for
// operator-facing UIs while flattening 95% of write amplification.
var apiKeyTouchCooldown atomic.Int64

// init seeds the process-wide touch coalescing interval. The value is kept in
// an atomic so tests can shorten it without racing hot API-key paths.
func init() {
	apiKeyTouchCooldown.Store(int64(60 * time.Second))
}

// apiKeyTouchSweepHighWater caps the in-memory throttle cache. Entries
// older than 2*cooldown are swept when the map crosses this size.
const apiKeyTouchSweepHighWater = 10_000

// apiKeyTouchCache stores the unix-nano timestamp of the last issued UPDATE
// per api-key id. A miss or a stale entry triggers the UPDATE. The map is
// process-global because Queries can be reconstructed mid-tx via withDB.
var apiKeyTouchCache sync.Map // map[string]int64

// apiKeyTouchSize tracks the number of distinct entries in apiKeyTouchCache.
// sync.Map has no O(1) Len; without this counter every touch would walk the
// map to decide whether to sweep, which defeats the throttling win at scale.
var apiKeyTouchSize atomic.Int64

// apiKeyTouchSweeping serializes sweepers so concurrent UPDATE returns over
// the high-water mark do not stampede the same eviction Range. Best-effort:
// any late callers fall through after the winner finishes.
var apiKeyTouchSweeping atomic.Bool

func (q *Queries) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAPIKey")
	defer span.End()

	if key.ID == "" {
		key.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO api_keys (id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at,
		                      environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, cache_version`

	var rotationWebhookSecret any
	if len(key.RotationWebhookSecret) > 0 {
		rotationWebhookSecret = key.RotationWebhookSecret
	}

	err := q.db.QueryRow(ctx, query,
		key.ID, key.ProjectID, dbscan.NilIfEmptyString(key.OrgID), key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt,
		dbscan.NilIfEmptyString(key.EnvironmentID), key.RotationIntervalDays, key.NextRotationAt, dbscan.NilIfEmptyString(key.RotationWebhookURL),
		rotationWebhookSecret,
	).Scan(&key.CreatedAt, &key.CacheVersion)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	return nil
}

func (q *Queries) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByHash")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys WHERE key_hash = $1`

	key, err := scanAPIKey(q.db.QueryRow(ctx, query, keyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return key, nil
}

func (q *Queries) ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysByProject")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`

	args := []any{projectID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) ListAPIKeysByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysByOrg")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys WHERE org_id = $1 AND revoked_at IS NULL`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys by org: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys by org scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) RevokeAPIKey(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RevokeAPIKey")
	defer span.End()

	query := `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found or already revoked")
	}

	evictAPIKeyTouch(id)
	return nil
}

func (q *Queries) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TouchAPIKeyLastUsed")
	defer span.End()

	cooldown := time.Duration(apiKeyTouchCooldown.Load())
	now := time.Now().UnixNano()
	if v, ok := apiKeyTouchCache.Load(id); ok {
		if last, ok := v.(int64); ok && now-last < int64(cooldown) {
			return nil
		}
	}

	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}

	recordAPIKeyTouch(id, now)
	sweepAPIKeyTouchCacheIfFull(cooldown)
	return nil
}

// evictAPIKeyTouch removes the throttle entry for id (if any) and
// decrements the size counter. Called from RevokeAPIKey so revoked keys
// do not occupy cache slots until the next high-water sweep — a revoked
// key will never legitimately call TouchAPIKeyLastUsed again, so its
// entry is wasted memory. LoadAndDelete is atomic, so concurrent revokes
// for the same id only decrement once.
func evictAPIKeyTouch(id string) {
	if _, loaded := apiKeyTouchCache.LoadAndDelete(id); loaded {
		apiKeyTouchSize.Add(-1)
	}
}

// recordAPIKeyTouch stores the latest touch timestamp for id, incrementing
// the size counter only when the entry is genuinely new. Concurrent first
// touches for the same id may race; LoadOrStore guarantees the counter is
// incremented at most once per surviving entry.
func recordAPIKeyTouch(id string, now int64) {
	if _, loaded := apiKeyTouchCache.LoadOrStore(id, now); loaded {
		apiKeyTouchCache.Store(id, now)
		return
	}
	apiKeyTouchSize.Add(1)
}

// sweepAPIKeyTouchCacheIfFull evicts entries older than 2*cooldown once the
// size counter crosses the high-water mark. The 2x window keeps
// recently-throttled keys around long enough to keep coalescing while
// bounding worst-case memory. A CAS guard ensures only one goroutine sweeps
// at a time; concurrent callers return immediately.
func sweepAPIKeyTouchCacheIfFull(cooldown time.Duration) {
	if apiKeyTouchSize.Load() <= apiKeyTouchSweepHighWater {
		return
	}
	if !apiKeyTouchSweeping.CompareAndSwap(false, true) {
		return
	}
	defer apiKeyTouchSweeping.Store(false)

	cutoff := time.Now().UnixNano() - int64(2*cooldown)
	var evicted int64
	apiKeyTouchCache.Range(func(k, v any) bool {
		last, ok := v.(int64)
		if ok && last >= cutoff {
			return true
		}
		// CompareAndDelete keeps eviction race-free: if a concurrent write
		// refreshed the entry after we observed it stale, we leave it.
		if apiKeyTouchCache.CompareAndDelete(k, v) {
			evicted++
		}
		return true
	})
	if evicted > 0 {
		apiKeyTouchSize.Add(-evicted)
	}
}

func (q *Queries) GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByID")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys WHERE id = $1`

	key, err := scanAPIKey(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return key, nil
}

func scanAPIKey(scanner scanTarget) (*domain.APIKey, error) {
	var key domain.APIKey
	var orgID *string
	var replacedBy *string
	var rateLimitRequests *int
	var rateLimitWindowSecs *int
	var environmentID *string
	var rotationWebhookURL *string
	var rotationWebhookSecret []byte
	err := scanner.Scan(
		&key.ID, &key.ProjectID, &orgID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt, &replacedBy, &key.GraceExpiresAt,
		&rateLimitRequests, &rateLimitWindowSecs,
		&environmentID, &key.RotationIntervalDays, &key.NextRotationAt, &rotationWebhookURL, &rotationWebhookSecret, &key.CacheVersion,
	)
	if err != nil {
		return nil, err
	}
	if orgID != nil {
		key.OrgID = *orgID
	}
	if replacedBy != nil {
		key.ReplacedByKeyID = *replacedBy
	}
	if rateLimitRequests != nil {
		key.RateLimitRequests = *rateLimitRequests
	}
	if rateLimitWindowSecs != nil {
		key.RateLimitWindowSecs = *rateLimitWindowSecs
	}
	if environmentID != nil {
		key.EnvironmentID = *environmentID
	}
	if rotationWebhookURL != nil {
		key.RotationWebhookURL = *rotationWebhookURL
	}
	key.RotationWebhookSecret = rotationWebhookSecret
	return &key, nil
}

func (q *Queries) ListAPIKeysDueRotation(ctx context.Context) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysDueRotation")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys
			  WHERE rotation_interval_days IS NOT NULL
			    AND next_rotation_at <= NOW()
			    AND revoked_at IS NULL
			    AND replaced_by_key_id IS NULL
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list api keys due rotation: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys due rotation scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

// DisableAPIKeyAutoRotation clears scheduler eligibility for a key whose
// auto-rotation configuration cannot be completed safely, such as legacy rows
// that have a rotation interval but no webhook URL to deliver the new secret.
func (q *Queries) DisableAPIKeyAutoRotation(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DisableAPIKeyAutoRotation")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		UPDATE api_keys
		SET next_rotation_at = NULL
		WHERE id = $1
		  AND revoked_at IS NULL
		  AND replaced_by_key_id IS NULL`, id)
	if err != nil {
		return fmt.Errorf("disable api key auto-rotation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (q *Queries) ListAPIKeysExpiringSoon(ctx context.Context, projectID string, withinDays int) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysExpiringSoon")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret, cache_version
			  FROM api_keys
			  WHERE project_id = $1
			    AND revoked_at IS NULL
			    AND (expires_at IS NULL OR expires_at <= NOW() + INTERVAL '1 day' * $2)
			  ORDER BY expires_at ASC NULLS FIRST
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query, projectID, withinDays)
	if err != nil {
		return nil, fmt.Errorf("list api keys expiring soon: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys expiring soon scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) ListRunsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByOrg")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs jr
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE jr.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
		           THEN (visibility.visible_until IS NULL OR visibility.visible_until > NOW())
		           ELSE (jr.visible_until IS NULL OR jr.visible_until > NOW())
		      END`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by org: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 8)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list runs by org scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	return runs, rows.Err()
}

func (q *Queries) ListJobsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobsByOrg")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
		       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs by org: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, 8)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list jobs by org scan: %w", scanErr)
		}
		jobs = append(jobs, *job)
	}

	return jobs, rows.Err()
}

func (q *Queries) MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkAPIKeyRotated")
	defer span.End()

	query := `
		UPDATE api_keys
		SET replaced_by_key_id = $2, grace_expires_at = $3
		WHERE id = $1 AND revoked_at IS NULL AND replaced_by_key_id IS NULL`
	tag, err := q.db.Exec(ctx, query, oldKeyID, newKeyID, graceExpiresAt)
	if err != nil {
		return fmt.Errorf("mark api key rotated: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found, already revoked, or already rotated")
	}
	return nil
}

func (q *Queries) CreateRotatedAPIKey(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRotatedAPIKey")
	defer span.End()

	if oldKeyID == "" {
		return fmt.Errorf("create rotated api key: old key id is required")
	}
	if newKey == nil {
		return fmt.Errorf("create rotated api key: new key is required")
	}
	if newKey.ID == "" {
		newKey.ID = uuid.Must(uuid.NewV7()).String()
	}

	if err := q.withTx(ctx, func(tx *Queries) error {
		if err := tx.CreateAPIKey(ctx, newKey); err != nil {
			return err
		}
		return tx.MarkAPIKeyRotated(ctx, oldKeyID, newKey.ID, graceExpiresAt)
	}); err != nil {
		return fmt.Errorf("create rotated api key: %w", err)
	}
	return nil
}
