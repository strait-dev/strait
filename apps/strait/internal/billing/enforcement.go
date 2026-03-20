package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"strait/internal/domain"

	"github.com/redis/go-redis/v9"
)

var (
	ErrOrgDailyRunLimitExceeded      = errors.New("org daily run limit exceeded")
	ErrOrgConcurrentRunLimitExceeded = errors.New("org concurrent run limit exceeded")
	ErrProjectLimitReached           = errors.New("project limit reached")
	ErrMemberLimitReached            = errors.New("member limit reached")
	ErrOrgLimitReached               = errors.New("org limit reached")
	ErrSpendingLimitReached          = errors.New("spending limit reached")
	ErrProjectBudgetReached          = errors.New("project budget reached")
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
	store    Store
	rdb      redis.Cmdable
	logger   *slog.Logger
	orgCache sync.Map // orgID -> cachedOrgLimits
	cacheTTL time.Duration
}

type cachedOrgLimits struct {
	tier      domain.PlanTier
	limits    OrgPlanLimits
	expiresAt time.Time
}

// NewEnforcer creates a billing enforcer.
func NewEnforcer(store Store, rdb redis.Cmdable, logger *slog.Logger) *Enforcer {
	return &Enforcer{
		store:    store,
		rdb:      rdb,
		logger:   logger,
		cacheTTL: 5 * time.Minute,
	}
}

// InvalidateOrgCache removes cached plan limits for an org (call on plan change).
func (e *Enforcer) InvalidateOrgCache(orgID string) {
	e.orgCache.Delete(orgID)
}

// PurgeExpiredCache removes expired entries from the org cache.
// Should be called periodically (e.g., every 10 minutes) from the scheduler.
func (e *Enforcer) PurgeExpiredCache() {
	now := time.Now()
	e.orgCache.Range(func(key, value any) bool {
		cached := value.(*cachedOrgLimits)
		if now.After(cached.expiresAt) {
			e.orgCache.Delete(key)
		}
		return true
	})
}

// GetOrgPlanLimits returns plan limits for an org, with caching.
func (e *Enforcer) GetOrgPlanLimits(ctx context.Context, orgID string) (OrgPlanLimits, error) {
	if orgID == "" {
		return GetPlanLimits(domain.PlanFree), nil
	}

	if entry, ok := e.orgCache.Load(orgID); ok {
		cached := entry.(*cachedOrgLimits)
		if time.Now().Before(cached.expiresAt) {
			return cached.limits, nil
		}
	}

	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			limits := GetPlanLimits(domain.PlanFree)
			e.orgCache.Store(orgID, &cachedOrgLimits{
				tier:      domain.PlanFree,
				limits:    limits,
				expiresAt: time.Now().Add(e.cacheTTL),
			})
			return limits, nil
		}
		return OrgPlanLimits{}, fmt.Errorf("getting org subscription: %w", err)
	}

	tier := domain.PlanTier(sub.PlanTier)
	limits := GetPlanLimits(tier)
	e.orgCache.Store(orgID, &cachedOrgLimits{
		tier:      tier,
		limits:    limits,
		expiresAt: time.Now().Add(e.cacheTTL),
	})
	return limits, nil
}

// CheckDailyRunLimit checks if the org has exceeded its daily run quota.
// Uses Redis INCR with TTL for atomic counting.
func (e *Enforcer) CheckDailyRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for run check", "org_id", orgID, "error", err)
		return nil // fail open
	}

	if limits.MaxRunsPerDay == -1 {
		return nil // unlimited
	}

	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01-02"))
	count, err := e.rdb.Incr(ctx, key).Result()
	if err != nil {
		e.logger.Warn("failed to increment org run counter", "org_id", orgID, "error", err)
		return nil // fail open
	}

	if count == 1 {
		if err := e.rdb.Expire(ctx, key, 48*time.Hour).Err(); err != nil {
			e.logger.Warn("failed to set TTL on org run counter", "org_id", orgID, "error", err)
		}
	}

	if count > limits.MaxRunsPerDay {
		if err := e.rdb.Decr(ctx, key).Err(); err != nil {
			e.logger.Warn("failed to rollback org run counter", "org_id", orgID, "error", err)
		}
		return &LimitError{
			Code:         "org_daily_run_limit_exceeded",
			Message:      fmt.Sprintf("Your %s plan allows %d runs per day. You've used %d.", limits.DisplayName, limits.MaxRunsPerDay, count-1),
			CurrentUsage: count - 1,
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

// CheckConcurrentRunLimit checks if the org has exceeded its concurrent run limit.
// Uses Redis INCR for atomic counting. Call DecrConcurrentRunCount when the run finishes.
func (e *Enforcer) CheckConcurrentRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
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
	count, err := e.rdb.Incr(ctx, key).Result()
	if err != nil {
		e.logger.Warn("failed to increment concurrent run counter", "org_id", orgID, "error", err)
		return nil // fail open
	}

	// Set TTL for self-healing (in case decrements are missed).
	if count == 1 {
		if err := e.rdb.Expire(ctx, key, concurrentCounterTTL).Err(); err != nil {
			e.logger.Warn("failed to set TTL on concurrent run counter", "org_id", orgID, "error", err)
		}
	}

	if count > int64(limits.MaxConcurrentRuns) {
		// Over limit: roll back the increment.
		if err := e.rdb.Decr(ctx, key).Err(); err != nil {
			e.logger.Warn("failed to rollback concurrent run counter", "org_id", orgID, "error", err)
		}
		return &LimitError{
			Code:         "org_concurrent_run_limit_exceeded",
			Message:      fmt.Sprintf("Your %s plan allows %d concurrent runs. Currently running: %d.", limits.DisplayName, limits.MaxConcurrentRuns, count-1),
			CurrentUsage: count - 1,
			Limit:        int64(limits.MaxConcurrentRuns),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

// DecrConcurrentRunCount decrements the concurrent run counter (call when a run finishes).
func (e *Enforcer) DecrConcurrentRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := fmt.Sprintf("strait:org_concurrent:%s", orgID)
	if err := e.rdb.Decr(ctx, key).Err(); err != nil {
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
		return nil // fail open
	}

	if budget < 0 {
		return nil // no budget set
	}

	periodStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	spend, err := e.store.GetProjectPeriodSpend(ctx, projectID, periodStart)
	if err != nil {
		e.logger.Warn("failed to get project period spend", "project_id", projectID, "error", err)
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

	// Reconcile each org in the union.
	for orgID := range orgs {
		actual, countErr := counter.CountExecutingRunsByOrg(ctx, orgID)
		if countErr != nil {
			e.logger.Warn("failed to count executing runs for reconciliation",
				"org_id", orgID, "error", countErr)
			continue
		}
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
