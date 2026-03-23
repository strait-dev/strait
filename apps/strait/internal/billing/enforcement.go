package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"strait/internal/cache/otterstore"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"
)

var (
	ErrOrgDailyRunLimitExceeded      = errors.New("org daily run limit exceeded")
	ErrOrgConcurrentRunLimitExceeded = errors.New("org concurrent run limit exceeded")
	ErrProjectLimitReached           = errors.New("project limit reached")
	ErrMemberLimitReached            = errors.New("member limit reached")
	ErrOrgLimitReached               = errors.New("org limit reached")
	ErrSpendingLimitReached          = errors.New("spending limit reached")
	ErrProjectBudgetReached          = errors.New("project budget reached")
	ErrGracePeriodExpired            = errors.New("payment grace period expired")
	ErrPaymentRestricted             = errors.New("payment restricted")
)

// LimitError provides structured information about a limit rejection.
type LimitError struct {
	Code         string `json:"error"`
	Message      string `json:"message"`
	CurrentUsage int64  `json:"current_usage"`
	Limit        int64  `json:"limit"`
	Plan         string `json:"plan"`
	UpgradeURL   string `json:"upgrade_url"`
}

func (e *LimitError) Error() string {
	return e.Message
}

// Enforcer checks org-level billing limits before allowing operations.
type Enforcer struct {
	store       Store
	rdb         redis.Cmdable
	logger      *slog.Logger
	metrics     *telemetry.Metrics
	orgCache    *cache.Cache[*cachedOrgLimits]
	limitsGroup singleflight.Group
	cacheTTL    time.Duration
}

type cachedOrgLimits struct {
	tier            domain.PlanTier
	limits          OrgPlanLimits
	enforcementMode string
}

// NewEnforcer creates a billing enforcer. Panics if store is nil.
func NewEnforcer(store Store, rdb redis.Cmdable, logger *slog.Logger, opts ...EnforcerOption) *Enforcer {
	if store == nil {
		panic("billing.NewEnforcer: store must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	cacheTTL := 5 * time.Minute
	cacheStore := otterstore.New(otterstore.Config{
		DefaultTTL:  cacheTTL,
		MaxCapacity: 1_000,
	})
	orgCache := cache.New[*cachedOrgLimits](cacheStore)

	e := &Enforcer{
		store:    store,
		rdb:      rdb,
		logger:   logger,
		orgCache: orgCache,
		cacheTTL: cacheTTL,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EnforcerOption configures the Enforcer.
type EnforcerOption func(*Enforcer)

// WithMetrics attaches Prometheus metrics to the enforcer.
func WithMetrics(m *telemetry.Metrics) EnforcerOption {
	return func(e *Enforcer) {
		e.metrics = m
	}
}

// InvalidateOrgCache removes cached plan limits for an org (call on plan change).
func (e *Enforcer) InvalidateOrgCache(orgID string) {
	_ = e.orgCache.Delete(context.Background(), orgID)
}

// getEnforcementMode returns the enforcement mode for an org from cache.
// Falls back to "enforce" if not cached.
func (e *Enforcer) getEnforcementMode(orgID string) string {
	if cached, err := e.orgCache.Get(context.Background(), orgID); err == nil {
		if cached.enforcementMode != "" {
			return cached.enforcementMode
		}
	}
	return "enforce"
}

// checkEnforcementMode returns true if enforcement is disabled or warn-only for
// the given org. Call this after GetOrgPlanLimits (which populates the cache).
func (e *Enforcer) checkEnforcementMode(orgID, checkType string) (skip bool) {
	mode := e.getEnforcementMode(orgID)
	switch mode {
	case "disabled":
		return true
	case "warn":
		e.logger.Warn("soft limit warning (enforcement_mode=warn)",
			"org_id", orgID, "check", checkType)
		return true
	default:
		return false
	}
}

// GetOrgPlanLimits returns plan limits for an org, with caching.
func (e *Enforcer) GetOrgPlanLimits(ctx context.Context, orgID string) (OrgPlanLimits, error) {
	if orgID == "" {
		return GetPlanLimits(domain.PlanFree), nil
	}

	if cached, err := e.orgCache.Get(ctx, orgID); err == nil {
		return cached.limits, nil
	}

	// Coalesce concurrent cache misses via singleflight to prevent
	// thundering herd on the DB when cache expires under load.
	result, err, _ := e.limitsGroup.Do(orgID, func() (any, error) {
		// Double-check cache inside singleflight (another goroutine may have populated it).
		if cached, err := e.orgCache.Get(ctx, orgID); err == nil {
			return cached.limits, nil
		}

		sub, err := e.store.GetOrgSubscription(ctx, orgID)
		if err != nil {
			if errors.Is(err, ErrSubscriptionNotFound) {
				limits := GetPlanLimits(domain.PlanFree)
				_ = e.orgCache.Set(ctx, orgID, &cachedOrgLimits{
					tier:            domain.PlanFree,
					limits:          limits,
					enforcementMode: "enforce",
				})
				return limits, nil
			}
			return OrgPlanLimits{}, fmt.Errorf("getting org subscription: %w", err)
		}

		return sub, nil
	})
	if err != nil {
		return OrgPlanLimits{}, err
	}

	// If singleflight returned cached limits directly, return them.
	if limits, ok := result.(OrgPlanLimits); ok {
		return limits, nil
	}

	// Otherwise, result is the OrgSubscription — build limits from it.
	sub := result.(*OrgSubscription)

	tier := domain.PlanTier(sub.PlanTier)
	limits := GetPlanLimits(tier)

	// Apply per-org overrides from support.
	if sub.OverrideDailyRunLimit != nil {
		limits.MaxRunsPerDay = int64(*sub.OverrideDailyRunLimit)
	}
	if sub.OverrideConcurrentRunLimit != nil {
		limits.MaxConcurrentRuns = *sub.OverrideConcurrentRunLimit
	}

	_ = e.orgCache.Set(ctx, orgID, &cachedOrgLimits{
		tier:            tier,
		limits:          limits,
		enforcementMode: sub.EnforcementMode,
	})
	return limits, nil
}

// checkPaymentStatus checks the org's payment/grace status. Returns
// (true, nil) if the caller should skip further limit checks (active grace),
// (false, nil) if normal enforcement should continue, or (false, err) if blocked.
func (e *Enforcer) checkPaymentStatus(ctx context.Context, orgID string) (bool, error) {
	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return false, nil // free tier, no payment status
		}
		e.logger.Warn("failed to get org subscription for payment check", "org_id", orgID, "error", err)
		e.recordFailOpen(ctx, "payment_status", "db_error")
		return false, nil // fail open
	}

	switch sub.PaymentStatus {
	case "restricted":
		return false, &LimitError{
			Code:    "payment_restricted",
			Message: "Your account is restricted due to failed payment. Please update your payment method.",
			Plan:    sub.PlanTier,
		}
	case "grace":
		if sub.GracePeriodEnd != nil && time.Now().Before(*sub.GracePeriodEnd) {
			// Active grace period: allow the run, skip further limit checks.
			return true, nil
		}
		// Grace period has expired.
		return false, &LimitError{
			Code:    "grace_period_expired",
			Message: "Your payment grace period has expired. Please update your payment method.",
			Plan:    sub.PlanTier,
		}
	default:
		return false, nil
	}
}

// CheckDailyRunLimit checks if the org has exceeded its daily run quota.
// Uses Redis INCR with TTL for atomic counting.
func (e *Enforcer) CheckDailyRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}

	if skipLimits, err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	} else if skipLimits {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for run check", "org_id", orgID, "error", err)
		e.recordFailOpen(ctx, "daily_run", "db_error")
		return nil // fail open
	}

	if e.checkEnforcementMode(orgID, "daily_run") {
		return nil
	}

	if limits.MaxRunsPerDay == -1 {
		return nil // unlimited
	}

	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01-02"))
	result, err := atomicIncrCheckScript.Run(ctx, e.rdb, []string{key},
		limits.MaxRunsPerDay, int(48*time.Hour/time.Second)).Result()
	if err != nil {
		e.logger.Warn("failed to run atomic daily run check", "org_id", orgID, "error", err)
		e.recordFailOpen(ctx, "daily_run", "redis_error")
		return nil // fail open
	}

	vals, ok := result.([]any)
	if !ok || len(vals) < 2 {
		e.logger.Warn("unexpected result from atomic daily run check", "org_id", orgID)
		e.recordFailOpen(ctx, "daily_run", "redis_error")
		return nil // fail open
	}

	allowed, _ := vals[0].(int64)
	currentCount, _ := vals[1].(int64)

	if allowed == 0 {
		e.recordRejection(ctx, "daily_run_limit", limits.PlanTier)
		return &LimitError{
			Code:         "org_daily_run_limit_exceeded",
			Message:      fmt.Sprintf("Your %s plan allows %d runs per day. You've used %d.", limits.DisplayName, limits.MaxRunsPerDay, currentCount),
			CurrentUsage: currentCount,
			Limit:        limits.MaxRunsPerDay,
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// DecrDailyRunCount decrements the daily run counter (for rollback on failure).
func (e *Enforcer) DecrDailyRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01-02"))
	if err := e.rdb.Decr(ctx, key).Err(); err != nil {
		e.logger.Warn("failed to decrement org run counter", "org_id", orgID, "error", err)
	}
}

// CheckManagedRunLimit checks if a free-tier org has exceeded its monthly managed
// execution cap. Paid plans use compute credits instead, so this only applies when
// FreeManagedRunsPerMonth > 0. Uses Redis INCR with monthly TTL.
func (e *Enforcer) CheckManagedRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}

	if skipLimits, err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	} else if skipLimits {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for managed run check", "org_id", orgID, "error", err)
		return nil
	}

	if limits.FreeManagedRunsPerMonth <= 0 {
		return nil // paid plan or unlimited
	}

	key := fmt.Sprintf("strait:org_managed_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01"))
	managedTTLSecs := int(35 * 24 * time.Hour / time.Second)
	result, err := atomicIncrCheckScript.Run(ctx, e.rdb, []string{key},
		limits.FreeManagedRunsPerMonth, managedTTLSecs).Result()
	if err != nil {
		e.logger.Warn("failed to run atomic managed run check", "org_id", orgID, "error", err)
		e.recordFailOpen(ctx, "managed_run", "redis_error")
		return nil // fail open
	}

	vals, ok := result.([]any)
	if !ok || len(vals) < 2 {
		e.logger.Warn("unexpected result from atomic managed run check", "org_id", orgID)
		e.recordFailOpen(ctx, "managed_run", "redis_error")
		return nil // fail open
	}

	allowed, _ := vals[0].(int64)
	currentCount, _ := vals[1].(int64)

	if allowed == 0 {
		e.recordRejection(ctx, "managed_run_limit", limits.PlanTier)
		return &LimitError{
			Code:         "managed_run_limit_exceeded",
			Message:      fmt.Sprintf("Your free plan allows %d managed runs per month. Upgrade for unlimited managed execution.", limits.FreeManagedRunsPerMonth),
			CurrentUsage: currentCount,
			Limit:        int64(limits.FreeManagedRunsPerMonth),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// DecrManagedRunCount decrements the monthly managed run counter (for rollback on failure).
func (e *Enforcer) DecrManagedRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := fmt.Sprintf("strait:org_managed_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01"))
	if err := e.rdb.Decr(ctx, key).Err(); err != nil {
		e.logger.Warn("failed to decrement managed run counter", "org_id", orgID, "error", err)
	}
}

// concurrentCheckScript is a Lua script that atomically increments the counter
// and checks the limit. Returns the new count if under limit, or -1 if at/over.
// KEYS[1] = counter key, ARGV[1] = max limit, ARGV[2] = TTL seconds.
var concurrentCheckScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
if count > tonumber(ARGV[1]) then
  redis.call('DECR', KEYS[1])
  return -1
end
return count
`)

// CheckConcurrentRunLimit checks if the org has exceeded its concurrent run limit.
// Uses a Lua script for atomic increment+check. Call DecrConcurrentRunCount when the run finishes.
func (e *Enforcer) CheckConcurrentRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}

	if skipLimits, err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	} else if skipLimits {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for concurrent check", "org_id", orgID, "error", err)
		return nil
	}

	if limits.MaxConcurrentRuns == -1 {
		return nil // unlimited
	}

	key := fmt.Sprintf("strait:org_concurrent:%s", orgID)
	result, err := concurrentCheckScript.Run(ctx, e.rdb, []string{key},
		limits.MaxConcurrentRuns,
		int(concurrentCounterTTL.Seconds()),
	).Int64()
	if err != nil {
		e.logger.Warn("failed to run concurrent check script", "org_id", orgID, "error", err)
		e.recordFailOpen(ctx, "concurrent_run", "redis_error")
		return nil // fail open
	}

	if result == -1 {
		// Script returned -1 meaning at/over limit (DECR already called).
		currentCount, _ := e.rdb.Get(ctx, key).Int64()
		e.recordRejection(ctx, "concurrent_limit", limits.PlanTier)
		return &LimitError{
			Code:         "org_concurrent_run_limit_exceeded",
			Message:      fmt.Sprintf("Your %s plan allows %d concurrent runs. Currently running: %d.", limits.DisplayName, limits.MaxConcurrentRuns, currentCount),
			CurrentUsage: currentCount,
			Limit:        int64(limits.MaxConcurrentRuns),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// atomicIncrCheckScript atomically increments a counter and checks against a limit.
// Returns {1, count} if allowed, {0, count} if over limit (counter is not incremented).
// ARGV[1] = limit (-1 for unlimited), ARGV[2] = TTL in seconds.
var atomicIncrCheckScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
local limit = tonumber(ARGV[1])
if limit ~= -1 and current > limit then
    redis.call('DECR', KEYS[1])
    return {0, current - 1}
end
if redis.call('TTL', KEYS[1]) == -1 then
    redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
end
return {1, current}
`)

// decrFloorScript decrements a counter but floors at zero to prevent negative values
// from double-decrements or decrements after reconciler resets.
var decrFloorScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current and tonumber(current) > 0 then
    return redis.call('DECR', KEYS[1])
end
return 0
`)

// DecrConcurrentRunCount decrements the concurrent run counter (call when a run finishes).
// Uses a Lua script to floor at zero, preventing negative values from double-decrements.
func (e *Enforcer) DecrConcurrentRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := fmt.Sprintf("strait:org_concurrent:%s", orgID)
	if err := decrFloorScript.Run(ctx, e.rdb, []string{key}).Err(); err != nil {
		e.logger.Warn("failed to decrement concurrent run counter", "org_id", orgID, "error", err)
	}
}

// CheckProjectLimit checks if org can create another project.
func (e *Enforcer) CheckProjectLimit(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for project check", "org_id", orgID, "error", err)
		return nil
	}

	if limits.MaxProjectsPerOrg == -1 {
		return nil
	}

	count, err := e.store.CountProjectsByOrg(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to count projects by org", "org_id", orgID, "error", err)
		return nil
	}

	if count >= limits.MaxProjectsPerOrg {
		e.recordRejection(ctx, "project_limit", limits.PlanTier)
		return &LimitError{
			Code:         "project_limit_reached",
			Message:      fmt.Sprintf("Your %s plan allows %d projects per organization. Upgrade to add more.", limits.DisplayName, limits.MaxProjectsPerOrg),
			CurrentUsage: int64(count),
			Limit:        int64(limits.MaxProjectsPerOrg),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// CheckSpendingLimit checks if the org has exceeded its spending limit.
func (e *Enforcer) CheckSpendingLimit(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}

	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil // free tier, no spending limit config
		}
		e.recordFailOpen(ctx, "spending_limit", "db_error")
		return nil // fail open
	}

	if sub.SpendingLimitMicrousd == -1 {
		return nil // no limit set
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	includedCredit := limits.ComputeCreditMicrousd

	periodStart := sub.CurrentPeriodStart
	if periodStart == nil {
		now := time.Now()
		periodStart = &now
	}

	periodSpend, err := e.store.SumOrgPeriodSpend(ctx, orgID, *periodStart)
	if err != nil {
		e.logger.Warn("failed to sum org period spend", "org_id", orgID, "error", err)
		return nil
	}

	overageSpend := max(periodSpend-includedCredit, 0)

	if overageSpend >= sub.SpendingLimitMicrousd {
		e.recordRejection(ctx, "spending_limit", limits.PlanTier)
		return &LimitError{
			Code:         "spending_limit_reached",
			Message:      fmt.Sprintf("Your monthly spending limit of $%.2f has been reached.", float64(sub.SpendingLimitMicrousd)/1000000),
			CurrentUsage: overageSpend,
			Limit:        sub.SpendingLimitMicrousd,
			Plan:         sub.PlanTier,
			UpgradeURL:   "/settings/billing",
		}
	}

	return nil
}

// CheckProjectBudgetLimit checks if a project has exceeded its monthly budget.
// Returns ErrProjectBudgetReached if the budget is exceeded and action is "reject".
// Fails open on any store errors.
func (e *Enforcer) CheckProjectBudgetLimit(ctx context.Context, projectID string) error {
	if projectID == "" {
		return nil
	}

	budget, action, err := e.store.GetProjectBudget(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to get project budget", "project_id", projectID, "error", err)
		e.recordFailOpen(ctx, "project_budget", "db_error")
		return nil // fail open
	}

	if budget < 0 {
		return nil // no budget set
	}

	periodStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	spend, err := e.store.GetProjectPeriodSpend(ctx, projectID, periodStart)
	if err != nil {
		e.logger.Warn("failed to get project period spend", "project_id", projectID, "error", err)
		e.recordFailOpen(ctx, "project_budget", "db_spend_error")
		return nil // fail open
	}

	if budget == 0 || spend >= budget {
		if action == "reject" {
			return &LimitError{
				Code:         "project_budget_reached",
				Message:      fmt.Sprintf("This project's monthly budget of $%.2f has been reached.", float64(budget)/1000000),
				CurrentUsage: spend,
				Limit:        budget,
				UpgradeURL:   "/settings/billing",
			}
		}
		e.logger.Warn("project budget reached (notify mode)",
			"project_id", projectID,
			"spend_microusd", spend,
			"budget_microusd", budget,
		)
	}

	return nil
}

// GetProjectOrgID resolves the org ID for a project via the billing store.
func (e *Enforcer) GetProjectOrgID(ctx context.Context, projectID string) (string, error) {
	return e.store.GetProjectOrgID(ctx, projectID)
}

// GetActiveProjectOrgID resolves the org ID for an active project via the billing store.
func (e *Enforcer) GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error) {
	return e.store.GetActiveProjectOrgID(ctx, projectID)
}

// ExecutingRunCounter provides ground-truth executing run counts from the database.
type ExecutingRunCounter interface {
	CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error)
	BulkCountExecutingRunsByOrg(ctx context.Context, orgIDs []string) (map[string]int, error)
	ListOrgsWithExecutingRuns(ctx context.Context) ([]string, error)
}

// ReconcileConcurrentRunCount sets the Redis concurrent run counter to the
// actual count from the database. This corrects drift caused by process crashes
// where DECR was never called after a run completed.
func (e *Enforcer) ReconcileConcurrentRunCount(ctx context.Context, orgID string, actualCount int) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}
	key := fmt.Sprintf("strait:org_concurrent:%s", orgID)
	if err := e.rdb.Set(ctx, key, actualCount, concurrentCounterTTL).Err(); err != nil {
		return fmt.Errorf("reconciling concurrent run count: %w", err)
	}
	return nil
}

// ReconcileAllConcurrentCounts reconciles Redis concurrent run counters with
// the actual count from the database. It uses the DB as source of truth:
// orgs with executing runs get their counter set to the real value, and stale
// Redis keys (for orgs with no executing runs) get reset to 0.
func (e *Enforcer) ReconcileAllConcurrentCounts(ctx context.Context, counter ExecutingRunCounter) error {
	if e.rdb == nil {
		return nil
	}

	// Build a union of org IDs from DB (executing runs) and Redis (existing keys).
	orgs := make(map[string]struct{})

	// Source 1: DB lists orgs that actually have executing runs.
	dbOrgs, err := counter.ListOrgsWithExecutingRuns(ctx)
	if err != nil {
		return fmt.Errorf("listing orgs with executing runs: %w", err)
	}
	for _, orgID := range dbOrgs {
		orgs[orgID] = struct{}{}
	}

	// Source 2: Redis SCAN finds orgs whose counters exist (may be stale).
	var cursor uint64
	pattern := "strait:org_concurrent:*"
	for {
		keys, nextCursor, scanErr := e.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if scanErr != nil {
			return fmt.Errorf("scanning concurrent counter keys: %w", scanErr)
		}
		for _, key := range keys {
			orgID := strings.TrimPrefix(key, "strait:org_concurrent:")
			orgs[orgID] = struct{}{}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// Bulk-fetch actual executing run counts for all orgs in one query.
	orgIDs := make([]string, 0, len(orgs))
	for orgID := range orgs {
		orgIDs = append(orgIDs, orgID)
	}

	counts, bulkErr := counter.BulkCountExecutingRunsByOrg(ctx, orgIDs)
	if bulkErr != nil {
		return fmt.Errorf("bulk counting executing runs for reconciliation: %w", bulkErr)
	}

	// Reconcile each org — orgs not in the counts map have 0 executing runs.
	for _, orgID := range orgIDs {
		actual := counts[orgID] // defaults to 0 if not in map
		if err := e.ReconcileConcurrentRunCount(ctx, orgID, actual); err != nil {
			e.logger.Warn("failed to reconcile concurrent count",
				"org_id", orgID, "error", err)
		}
	}

	return nil
}

// concurrentCounterTTL is the TTL for concurrent run counters.
// The reconciler runs every 5 minutes to correct drift; 24h is a backstop
// for total Redis failure. Managed runs can last many hours, so shorter
// TTLs cause keys to expire mid-run and undercount active concurrency.
const concurrentCounterTTL = 24 * time.Hour

// GetDailyRunCount returns the current daily run count for an org.
func (e *Enforcer) GetDailyRunCount(ctx context.Context, orgID string) (int64, error) {
	if orgID == "" || e.rdb == nil {
		return 0, nil
	}
	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01-02"))
	count, err := e.rdb.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("getting daily run count: %w", err)
	}
	return count, nil
}

// recordRejection increments the limit rejection Prometheus counter.
func (e *Enforcer) recordRejection(ctx context.Context, reason string, planTier domain.PlanTier) {
	if e.metrics == nil || e.metrics.LimitRejections == nil {
		return
	}
	e.metrics.LimitRejections.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("reason", reason),
			attribute.String("plan_tier", string(planTier)),
		),
	)
}

// recordFailOpen increments the fail-open Prometheus counter for ops visibility.
func (e *Enforcer) recordFailOpen(ctx context.Context, checkType, errorType string) {
	if e.metrics == nil || e.metrics.EnforcementFailOpen == nil {
		return
	}
	e.metrics.EnforcementFailOpen.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("check_type", checkType),
			attribute.String("error_type", errorType),
		),
	)
}

// CheckMemberLimit checks if the org can add another member.
func (e *Enforcer) CheckMemberLimit(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for member check", "org_id", orgID, "error", err)
		return nil
	}

	if limits.MaxMembersPerOrg == -1 {
		return nil
	}

	count, err := e.store.CountMembersByOrg(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to count members by org", "org_id", orgID, "error", err)
		return nil
	}

	if count >= limits.MaxMembersPerOrg {
		e.recordRejection(ctx, "member_limit", limits.PlanTier)
		return &LimitError{
			Code:         "member_limit_reached",
			Message:      fmt.Sprintf("Your %s plan allows %d members per organization. Upgrade to add more.", limits.DisplayName, limits.MaxMembersPerOrg),
			CurrentUsage: int64(count),
			Limit:        int64(limits.MaxMembersPerOrg),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// CheckOrgCreationLimit checks if the user can create another organization.
func (e *Enforcer) CheckOrgCreationLimit(ctx context.Context, userID string, planTier domain.PlanTier) error {
	if userID == "" {
		return nil
	}

	limits := GetPlanLimits(planTier)

	if limits.MaxOrgsPerUser == -1 {
		return nil
	}

	count, err := e.store.CountOrgsByUser(ctx, userID)
	if err != nil {
		e.logger.Warn("failed to count orgs by user", "user_id", userID, "error", err)
		return nil
	}

	if count >= limits.MaxOrgsPerUser {
		e.recordRejection(ctx, "org_limit", limits.PlanTier)
		return &LimitError{
			Code:         "org_limit_reached",
			Message:      fmt.Sprintf("Your %s plan allows %d organizations. Upgrade to create more.", limits.DisplayName, limits.MaxOrgsPerUser),
			CurrentUsage: int64(count),
			Limit:        int64(limits.MaxOrgsPerUser),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// Check80PercentDailyRunWarning returns true if the org has used 80% or more
// of its daily run limit. Returns false for unlimited plans or on error.
func (e *Enforcer) Check80PercentDailyRunWarning(ctx context.Context, orgID string) (bool, error) {
	if orgID == "" || e.rdb == nil {
		return false, nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("getting org plan limits: %w", err)
	}

	if limits.MaxRunsPerDay == -1 {
		return false, nil
	}

	count, err := e.GetDailyRunCount(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("getting daily run count: %w", err)
	}

	threshold := int64(float64(limits.MaxRunsPerDay) * 0.8)
	return count >= threshold, nil
}

// EnsureOrgSubscription delegates to the underlying store to lazily
// initialize a free-tier subscription row for the given org.
func (e *Enforcer) EnsureOrgSubscription(ctx context.Context, orgID string) error {
	return e.store.EnsureOrgSubscription(ctx, orgID)
}
