package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// ExecutingRunCounter counts executing runs for an org across all projects.
type ExecutingRunCounter interface {
	CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error)
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
func (e *Enforcer) CheckConcurrentRunLimit(ctx context.Context, orgID string, counter ExecutingRunCounter) error {
	if orgID == "" {
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

	executing, err := counter.CountExecutingRunsByOrg(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to count executing runs by org", "org_id", orgID, "error", err)
		return nil // fail open
	}

	if executing >= limits.MaxConcurrentRuns {
		return &LimitError{
			Code:         "org_concurrent_run_limit_exceeded",
			Message:      fmt.Sprintf("Your %s plan allows %d concurrent runs. Currently running: %d.", limits.DisplayName, limits.MaxConcurrentRuns, executing),
			CurrentUsage: int64(executing),
			Limit:        int64(limits.MaxConcurrentRuns),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
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

// GetProjectOrgID resolves the org ID for a project via the billing store.
func (e *Enforcer) GetProjectOrgID(ctx context.Context, projectID string) (string, error) {
	return e.store.GetProjectOrgID(ctx, projectID)
}

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
