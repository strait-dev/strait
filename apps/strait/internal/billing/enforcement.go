package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
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
	ErrDispatchPriorityExceeded      = errors.New("dispatch priority exceeds plan cap")
	ErrPaymentRestricted             = errors.New("payment restricted")
	ErrProjectSuspended              = errors.New("project suspended due to plan downgrade")
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
	store           Store
	rdb             redis.Cmdable
	logger          *slog.Logger
	metrics         *telemetry.Metrics
	orgCache        *straitcache.Tier[string, *cachedOrgLimits]
	cacheBus        *straitcache.Bus
	cacheRegistry   *straitcache.Registry
	limitsGroup     singleflight.Group
	cacheTTL        time.Duration
	suspendedCache  sync.Map // projectID -> *suspendedCacheEntry
	chExporter      billingEventEnqueuer
	failOpenTracker sync.Map // "orgID:checkType" -> *failOpenEntry
	billingEmails   *BillingEmailSender
	sentryMode      string
	sentryRegion    string
	sentryVersion   string
	requireRedis    bool
	bgWG            conc.WaitGroup
	// entitlementsAuthoritative controls whether GetOrgPlanLimits reads the
	// persisted snapshot directly when present. When false, it always recomputes
	// from the catalog + addons pipeline. Operator escape hatch via
	// BILLING_ENTITLEMENTS_AUTHORITATIVE.
	entitlementsAuthoritative bool

	// billingDispatcher fans billing-state events out to the outbound
	// webhook_subscriptions pipeline. nil means dispatch is disabled
	// (community edition, tests, or unwired cloud).
	billingDispatcher BillingEventDispatcher
}

const (
	maxConsecutiveFailOpen = 5
	failOpenWindow         = 30 * time.Second
)

type failOpenEntry struct {
	count     atomic.Int64
	firstSeen atomic.Int64 // unix nanos
}

// boundedFailOpen tracks consecutive fail-open events per org+check.
// Returns nil if under the threshold (allow the request), or a LimitError
// if the threshold is exceeded within the time window (fail-closed).
func (e *Enforcer) boundedFailOpen(ctx context.Context, orgID, checkType, reason string) error {
	e.recordFailOpen(ctx, checkType, reason)

	key := orgID + ":" + checkType
	entry, _ := e.failOpenTracker.LoadOrStore(key, &failOpenEntry{})
	fe := entry.(*failOpenEntry)

	now := time.Now().UnixNano()
	first := fe.firstSeen.Load()
	if first == 0 || time.Duration(now-first) > failOpenWindow {
		fe.firstSeen.Store(now)
		fe.count.Store(1)
		return nil
	}

	count := fe.count.Add(1)
	if count > maxConsecutiveFailOpen {
		e.logger.Warn("fail-open threshold exceeded, failing closed",
			"org_id", orgID, "check_type", checkType, "count", count)
		return &LimitError{
			Code:    "service_degraded",
			Message: "Billing enforcement is temporarily unavailable. Please retry shortly.",
		}
	}
	return nil
}

func (e *Enforcer) failClosedPlanLimitLookup(ctx context.Context, orgID, checkType string, err error) error {
	e.recordRejection(ctx, checkType, domain.PlanFree)
	addBillingSentryBreadcrumb(ctx, checkType, "billing plan limit lookup failed closed", map[string]any{
		"org_id": orgID,
		"error":  err.Error(),
	})
	return &LimitError{
		Code:    "billing_plan_unavailable",
		Message: "Billing enforcement is temporarily unavailable. Please retry shortly.",
		Plan:    string(domain.PlanFree),
	}
}

func serviceDegradedLimitError() *LimitError {
	return &LimitError{
		Code:    "service_degraded",
		Message: "Billing enforcement is temporarily unavailable. Please retry shortly.",
	}
}

// resetFailOpen clears the fail-open tracker for a successful check.
func (e *Enforcer) resetFailOpen(orgID, checkType string) {
	e.failOpenTracker.Delete(orgID + ":" + checkType)
}

// startFailOpenCleanup periodically removes stale entries from the fail-open tracker.
func (e *Enforcer) startFailOpenCleanup(ctx context.Context) {
	var wg conc.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(failOpenWindow * 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UnixNano()
				e.failOpenTracker.Range(func(key, value any) bool {
					fe := value.(*failOpenEntry)
					first := fe.firstSeen.Load()
					if first > 0 && time.Duration(now-first) > failOpenWindow*2 {
						e.failOpenTracker.Delete(key)
					}
					return true
				})
			}
		}
	})
}

// StartCleanup starts background cleanup goroutines for bounded caches.
// Call this after creating the enforcer. The goroutines stop when ctx is canceled.
func (e *Enforcer) StartCleanup(ctx context.Context) {
	e.startFailOpenCleanup(ctx)
}

// billingEventEnqueuer is the subset of clickhouse.Exporter needed for billing analytics.
type billingEventEnqueuer interface {
	Enqueue(record any) bool
}

type cachedOrgLimits struct {
	tier            domain.PlanTier
	limits          OrgPlanLimits
	enforcementMode string
}

type cachedOrgLimitsJSON struct {
	Tier            domain.PlanTier `json:"tier"`
	Limits          OrgPlanLimits   `json:"limits"`
	EnforcementMode string          `json:"enforcement_mode"`
}

func (c cachedOrgLimits) MarshalJSON() ([]byte, error) {
	return json.Marshal(cachedOrgLimitsJSON{
		Tier:            c.tier,
		Limits:          c.limits,
		EnforcementMode: c.enforcementMode,
	})
}

func (c *cachedOrgLimits) UnmarshalJSON(data []byte) error {
	var wire cachedOrgLimitsJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	c.tier = wire.Tier
	c.limits = wire.Limits
	c.enforcementMode = wire.EnforcementMode
	return nil
}

const orgLimitsCacheNamespace = "billing_org_limits"

// NewEnforcer creates a billing enforcer. Panics if store is nil.
func NewEnforcer(store Store, rdb redis.Cmdable, logger *slog.Logger, opts ...EnforcerOption) *Enforcer {
	if store == nil {
		panic("billing.NewEnforcer: store must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	cacheTTL := 5 * time.Minute
	e := &Enforcer{
		store:                     store,
		rdb:                       rdb,
		logger:                    logger,
		cacheTTL:                  cacheTTL,
		entitlementsAuthoritative: true,
	}
	for _, opt := range opts {
		opt(e)
	}
	var l2 straitcache.L2[string, *cachedOrgLimits]
	if rdb != nil {
		l2 = straitcache.NewRedisL2[string, *cachedOrgLimits](straitcache.RedisL2Config[string, *cachedOrgLimits]{
			Client:    rdb,
			Namespace: orgLimitsCacheNamespace,
		})
	}
	orgCache := straitcache.NewTier[string, *cachedOrgLimits](straitcache.TierConfig[string, *cachedOrgLimits]{
		Name:        orgLimitsCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		MaximumSize: 1_000,
		TTL:         cacheTTL,
		TTLJitter:   0.1,
		DisableL1:   l2 != nil,
		DisableL2:   l2 == nil,
		Clone: func(limits *cachedOrgLimits) *cachedOrgLimits {
			if limits == nil {
				return nil
			}
			clone := *limits
			clone.limits.AllowedRegions = append([]string(nil), limits.limits.AllowedRegions...)
			if limits.limits.MaxAddonPacks != nil {
				clone.limits.MaxAddonPacks = make(map[AddonType]int, len(limits.limits.MaxAddonPacks))
				maps.Copy(clone.limits.MaxAddonPacks, limits.limits.MaxAddonPacks)
			}
			return &clone
		},
	})
	e.orgCache = orgCache
	if e.cacheRegistry != nil {
		handler := straitcache.UpdatingStringTierHandler[*cachedOrgLimits]{Tier: orgCache}
		e.cacheRegistry.Register(orgLimitsCacheNamespace, handler)
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

func WithCacheBus(bus *straitcache.Bus, registry *straitcache.Registry) EnforcerOption {
	return func(e *Enforcer) {
		e.cacheBus = bus
		e.cacheRegistry = registry
	}
}

// WithClickHouse attaches a ClickHouse exporter for billing analytics events.
func WithClickHouse(exporter billingEventEnqueuer) EnforcerOption {
	return func(e *Enforcer) {
		e.chExporter = exporter
	}
}

// WithEnforcerBillingEmails attaches a billing email sender for spending alerts.
func WithEnforcerBillingEmails(sender *BillingEmailSender) EnforcerOption {
	return func(e *Enforcer) { e.billingEmails = sender }
}

// WithRequireRedis makes Redis-backed limit gates fail closed when the
// enforcer has no Redis client. Use this when cloud billing enforcement is
// enabled; community and webhook-only paths can keep the default no-op
// behavior for Redis-backed counters.
func WithRequireRedis() EnforcerOption {
	return func(e *Enforcer) {
		e.requireRedis = true
	}
}

// WithEntitlementsAuthoritative toggles whether the Enforcer reads the
// persisted entitlements snapshot on the hot path (true) or always
// recomputes from the catalog + addons pipeline (false).
func WithEntitlementsAuthoritative(b bool) EnforcerOption {
	return func(e *Enforcer) {
		e.entitlementsAuthoritative = b
	}
}

// WithBillingDispatcher attaches an outbound billing-event dispatcher
// to the Enforcer. When set, spending-cap and overage-state transitions
// dispatch the corresponding billing.* webhook events.
func WithBillingDispatcher(d BillingEventDispatcher) EnforcerOption {
	return func(e *Enforcer) {
		e.billingDispatcher = d
	}
}

// WithSentryRuntime attaches low-cardinality runtime tags to billing capture paths.
func WithSentryRuntime(mode, region, version string) EnforcerOption {
	return func(e *Enforcer) {
		e.sentryMode = mode
		e.sentryRegion = region
		e.sentryVersion = version
	}
}

// shouldSendBillingEmail checks if a billing email should be sent (24h cooldown per org+type).
func (e *Enforcer) shouldSendBillingEmail(ctx context.Context, orgID, emailType string) bool {
	if e.rdb == nil {
		return true
	}
	key := "strait:billing_email:" + orgID + ":" + emailType
	set, err := e.rdb.SetNX(ctx, key, "1", 24*time.Hour).Result()
	if err != nil {
		return true // fail open
	}
	return set
}

// EmitBillingEvent is the public wrapper around emitBillingEvent. Background
// jobs (downgrade applier, schedulers) call this to record overage signals
// like org_member_overage that the billing dashboard surfaces.
func (e *Enforcer) EmitBillingEvent(orgID, eventType, planTier string) {
	e.emitBillingEvent(orgID, eventType, planTier)
}

// emitBillingEvent sends a billing analytics event to ClickHouse.
func (e *Enforcer) emitBillingEvent(orgID, eventType, planTier string) {
	if e.chExporter == nil {
		return
	}
	e.chExporter.Enqueue(clickhouse.BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     orgID,
		EventType: eventType,
		PlanTier:  planTier,
	})
}

// DispatchBilling sends a billing webhook event through the configured
// BillingEventDispatcher. When no dispatcher is wired (community edition
// or test setup) the call is a silent no-op so call sites do not have to
// nil-guard. Errors are logged but never propagated so a webhook delivery
// failure cannot block the enforcement path that scheduled it.
func (e *Enforcer) DispatchBilling(ctx context.Context, orgID string, planTier domain.PlanTier, eventType string, detail map[string]any) {
	if e == nil || e.billingDispatcher == nil || orgID == "" || eventType == "" {
		return
	}
	if err := DispatchBillingWebhook(ctx, e.billingDispatcher, orgID, planTier, eventType, detail); err != nil {
		e.logger.Warn("dispatch billing webhook failed",
			"org_id", orgID, "event_type", eventType, "error", err)
	}
}

// tryDispatchCapEvent serializes one cap-event dispatch per billing period
// using the per-event dedup column on organization_subscriptions. When the
// caller is the first to mark the column in the current period, the event
// is dispatched; otherwise the call is a no-op.
func (e *Enforcer) tryDispatchCapEvent(
	ctx context.Context,
	orgID string,
	planTier domain.PlanTier,
	ev BillingCapEvent,
	eventType string,
	detail map[string]any,
) {
	if e == nil || e.billingDispatcher == nil || orgID == "" {
		return
	}
	first, err := e.store.TryMarkBillingCapEvent(ctx, orgID, ev)
	if err != nil {
		e.logger.Warn("mark billing cap event failed",
			"org_id", orgID, "event_type", eventType, "error", err)
		return
	}
	if !first {
		return
	}
	e.DispatchBilling(ctx, orgID, planTier, eventType, detail)
}

// InvalidateOrgCache removes cached plan limits for an org (call on plan change).
func (e *Enforcer) InvalidateOrgCache(orgID string) {
	e.InvalidateOrgCacheContext(context.Background(), orgID)
}

func (e *Enforcer) InvalidateOrgCacheContext(ctx context.Context, orgID string) {
	e.InvalidateOrgCacheWithVersionContext(ctx, orgID, time.Now().UnixNano())
}

// InvalidateOrgCacheWithVersion removes cached plan limits behind a version barrier.
func (e *Enforcer) InvalidateOrgCacheWithVersion(orgID string, version int64) {
	e.InvalidateOrgCacheWithVersionContext(context.Background(), orgID, version)
}

func (e *Enforcer) InvalidateOrgCacheWithVersionContext(ctx context.Context, orgID string, version int64) {
	if e == nil || e.orgCache == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = e.orgCache.StrongInvalidate(
		ctx,
		straitcache.StrongNamespacePolicy{Namespace: orgLimitsCacheNamespace},
		orgID,
		orgID,
		straitcache.VersionBarrier{Version: version},
		e.cacheBus,
	)
}

// getEnforcementMode returns the enforcement mode for an org from cache.
// Falls back to "enforce" if not cached.
func (e *Enforcer) getEnforcementMode(ctx context.Context, orgID string) string {
	if e == nil || e.orgCache == nil {
		return "enforce"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cached, err := e.orgCache.Get(ctx, orgID, nil); err == nil && cached != nil {
		if cached.enforcementMode != "" {
			return cached.enforcementMode
		}
	}
	return "enforce"
}

// checkEnforcementMode returns true if enforcement is disabled or warn-only for
// the given org. Call this after GetOrgPlanLimits (which populates the cache).
func (e *Enforcer) checkEnforcementMode(ctx context.Context, orgID, checkType string) (skip bool) {
	mode := e.getEnforcementMode(ctx, orgID)
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
func (e *Enforcer) GetOrgPlanLimits(ctx context.Context, orgID string) (limits OrgPlanLimits, retErr error) {
	ctx = telemetry.EnsureSentryHub(ctx)
	if e == nil || orgID == "" {
		return GetPlanLimits(domain.PlanFree), nil
	}
	addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limits requested", map[string]any{
		"org_id": orgID,
	})

	// Guard against nil orgCache or uninitialized otter internals that can
	// panic (observed when the billing enforcer is created without a fully
	// functional backing store, e.g. missing Stripe configuration).
	defer func() {
		if r := recover(); r != nil {
			addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limit panic", map[string]any{
				"org_id": orgID,
				"panic":  fmt.Sprintf("%v", r),
			})
			if hub := sentry.GetHubFromContext(ctx); hub != nil {
				hub.WithScope(func(scope *sentry.Scope) {
					e.applyBillingSentryScope(scope, orgID, "plan_limits")
					scope.SetLevel(sentry.LevelError)
					scope.SetContext("billing", sentry.Context{
						"org_id":    orgID,
						"operation": "plan_limits",
						"panic":     fmt.Sprintf("%v", r),
					})
					hub.Recover(r)
				})
			}
			slog.Error("recovered panic in GetOrgPlanLimits, returning free-tier defaults",
				"org_id", orgID, "panic", r)
			limits = GetPlanLimits(domain.PlanFree)
			retErr = nil
		}
	}()

	if e.orgCache == nil {
		addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limits cache unavailable", map[string]any{
			"org_id": orgID,
		})
		return GetPlanLimits(domain.PlanFree), nil
	}

	if cached, err := e.orgCache.Get(ctx, orgID, nil); err == nil && cached != nil {
		addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limits cache hit", map[string]any{
			"org_id": orgID,
			"plan":   string(cached.limits.PlanTier),
		})
		return cached.limits, nil
	}

	// Coalesce concurrent cache misses via singleflight to prevent
	// thundering herd on the DB when cache expires under load.
	result, err, _ := e.limitsGroup.Do(orgID, func() (any, error) {
		// Double-check cache inside singleflight (another goroutine may have populated it).
		if cached, err := e.orgCache.Get(ctx, orgID, nil); err == nil && cached != nil {
			return cached.limits, nil
		}

		sub, err := e.store.GetOrgSubscription(ctx, orgID)
		if err != nil {
			if errors.Is(err, ErrSubscriptionNotFound) {
				limits := GetPlanLimits(domain.PlanFree)
				cached := &cachedOrgLimits{
					tier:            domain.PlanFree,
					limits:          limits,
					enforcementMode: "enforce",
				}
				_, _ = e.orgCache.StrongWriteThrough(
					ctx,
					straitcache.StrongNamespacePolicy{Namespace: orgLimitsCacheNamespace},
					orgID,
					orgID,
					cached,
					1,
					e.cacheBus,
				)
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
	resolution, err := e.resolveOrgPlanLimits(ctx, orgID, sub)
	if err != nil {
		return OrgPlanLimits{}, err
	}

	cached := &cachedOrgLimits{
		tier:            resolution.tier,
		limits:          resolution.limits,
		enforcementMode: resolution.enforcementMode,
	}
	_, _ = e.orgCache.StrongWriteThrough(
		ctx,
		straitcache.StrongNamespacePolicy{Namespace: orgLimitsCacheNamespace},
		orgID,
		orgID,
		cached,
		resolution.cacheVersion,
		e.cacheBus,
	)
	return resolution.limits, nil
}

// checkPaymentStatus checks the org's payment/grace status. nil means normal
// plan enforcement should continue. Active grace allows payment access but
// must not skip normal quota enforcement.
func (e *Enforcer) checkPaymentStatus(ctx context.Context, orgID string) error {
	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil // free tier, no payment status
		}
		e.logger.Warn("failed to get org subscription for payment check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	switch sub.PaymentStatus {
	case "restricted":
		return &LimitError{
			Code:    "payment_restricted",
			Message: "Your account is restricted due to failed payment. Please update your payment method.",
			Plan:    sub.PlanTier,
		}
	case "suspended":
		return &LimitError{
			Code:    "payment_suspended",
			Message: "Your account is suspended due to failed payment. Please update your payment method.",
			Plan:    sub.PlanTier,
		}
	case "grace":
		if sub.GracePeriodEnd != nil && time.Now().Before(*sub.GracePeriodEnd) {
			return nil
		}
		// Grace period has expired.
		return &LimitError{
			Code:    "grace_period_expired",
			Message: "Your payment grace period has expired. Please update your payment method.",
			Plan:    sub.PlanTier,
		}
	default:
		return nil
	}
}

// CheckDailyRunLimit checks if the org has exceeded its daily run quota.
// Uses Redis INCR with TTL for atomic counting.
func (e *Enforcer) CheckDailyRunLimit(ctx context.Context, orgID string) error {
	if orgID == "" || e.rdb == nil {
		return nil
	}

	if err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for run check", "org_id", orgID, "error", err)
		return e.failClosedPlanLimitLookup(ctx, orgID, "daily_run", err)
	}
	e.resetFailOpen(orgID, "daily_run")

	if e.checkEnforcementMode(ctx, orgID, "daily_run") {
		return nil
	}

	if limits.MaxRunsPerDay == -1 {
		return nil // unlimited
	}

	period := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, period)
	result, err := atomicIncrCheckScript.Run(ctx, e.rdb, []string{key},
		limits.MaxRunsPerDay, int(48*time.Hour/time.Second)).Result()
	if err != nil {
		e.logger.Warn("failed to run atomic daily run check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	vals, ok := result.([]any)
	if !ok || len(vals) < 2 {
		e.logger.Warn("unexpected result from atomic daily run check", "org_id", orgID)
		return serviceDegradedLimitError()
	}

	allowed, _ := vals[0].(int64)
	currentCount, _ := vals[1].(int64)
	recordBillingQuotaUsage(ctx, "daily_runs", string(limits.PlanTier), quotaUsageRatio(currentCount, limits.MaxRunsPerDay))

	e.maybeEmitUsageThreshold(ctx, orgID, string(limits.PlanTier), "daily_runs", period,
		currentCount, limits.MaxRunsPerDay)

	if allowed == 0 {
		// Paid plans (Starter/Pro/Enterprise) allow overage — log but don't reject.
		// Overage is tracked via Stripe metered billing.
		if limits.PlanTier != domain.PlanFree {
			e.logger.Info("daily run limit exceeded on paid plan (overage allowed)",
				"org_id", orgID,
				"plan", limits.DisplayName,
				"limit", limits.MaxRunsPerDay,
				"current", currentCount,
			)
			recordBillingOverageEntered(ctx, string(limits.PlanTier))
			recordBillingOverageRun(ctx, "daily_runs", string(limits.PlanTier))
			return nil
		}

		// Free tier: hard reject.
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
// Uses decrFloorScript to prevent negative values from double-decrements.
func (e *Enforcer) DecrDailyRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := fmt.Sprintf("strait:org_runs:%s:%s", orgID, time.Now().UTC().Format("2006-01-02"))
	if err := decrFloorScript.Run(ctx, e.rdb, []string{key}).Err(); err != nil {
		e.logger.Warn("failed to decrement org run counter", "org_id", orgID, "error", err)
	}
}

// monthlyRunKey returns the Redis key for the org's monthly run counter.
// Key is scoped to the calendar month (UTC) and expires after 62 days.
func monthlyRunKey(orgID string, t time.Time) string {
	return fmt.Sprintf("strait:org_monthly_runs:%s:%s", orgID, t.UTC().Format("2006-01"))
}

// DecrMonthlyRunCount decrements the monthly run counter (for rollback on failure).
// Uses decrFloorScript to prevent negative values from double-decrements.
// Call this on any enqueue-abort path that happened AFTER CheckMonthlyRunLimit incremented
// the counter but BEFORE the run was successfully persisted.
func (e *Enforcer) DecrMonthlyRunCount(ctx context.Context, orgID string) {
	if orgID == "" || e.rdb == nil {
		return
	}
	key := monthlyRunKey(orgID, time.Now())
	if err := decrFloorScript.Run(ctx, e.rdb, []string{key}).Err(); err != nil {
		e.logger.Warn("failed to decrement org monthly run counter", "org_id", orgID, "error", err)
	}
}

const monthlyRunCounterTTLSecs = int(62 * 24 * time.Hour / time.Second)

// CheckMonthlyRunLimit checks if the org has exceeded its monthly run quota.
// Orgs with overage enabled enter metered overage; orgs with overage disabled
// are hard-capped at the included monthly allowance.
// Returns a *LimitError with code "plan_cap_reached" when the free cap is hit.
func (e *Enforcer) CheckMonthlyRunLimit(ctx context.Context, orgID string) error {
	return e.checkMonthlyRunLimit(ctx, orgID, "")
}

// CheckMonthlyRunLimitForRun checks the monthly quota and records whether this
// specific run is billable overage. Stripe metering consumes that marker after
// the run completes so included-allowance runs are never reported as overage.
func (e *Enforcer) CheckMonthlyRunLimitForRun(ctx context.Context, orgID, runID string) error {
	return e.checkMonthlyRunLimit(ctx, orgID, runID)
}

func (e *Enforcer) checkMonthlyRunLimit(ctx context.Context, orgID, runID string) error {
	if orgID == "" {
		return nil
	}
	if e.rdb == nil {
		if e.requireRedis {
			e.logger.Warn("monthly run limit unavailable: Redis client not configured", "org_id", orgID)
			return serviceDegradedLimitError()
		}
		return nil
	}

	if err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for monthly run check", "org_id", orgID, "error", err)
		return e.failClosedPlanLimitLookup(ctx, orgID, "monthly_run", err)
	}
	e.resetFailOpen(orgID, "monthly_run")

	if e.checkEnforcementMode(ctx, orgID, "monthly_run") {
		return nil
	}

	if limits.MaxRunsPerMonth == -1 {
		return nil // unlimited
	}

	period := time.Now().UTC().Format("2006-01")
	key := monthlyRunKey(orgID, time.Now())
	result, err := atomicIncrCheckScript.Run(ctx, e.rdb, []string{key},
		int64(limits.MaxRunsPerMonth), int64(monthlyRunCounterTTLSecs)).Result()
	if err != nil {
		e.logger.Warn("failed to run atomic monthly run check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	vals, ok := result.([]any)
	if !ok || len(vals) < 2 {
		e.logger.Warn("unexpected result from atomic monthly run check", "org_id", orgID)
		return serviceDegradedLimitError()
	}

	allowed, _ := vals[0].(int64)
	currentCount, _ := vals[1].(int64)
	recordBillingQuotaUsage(ctx, "monthly_runs", string(limits.PlanTier), quotaUsageRatio(currentCount, int64(limits.MaxRunsPerMonth)))

	e.maybeEmitUsageThreshold(ctx, orgID, string(limits.PlanTier), "monthly_runs", period,
		currentCount, int64(limits.MaxRunsPerMonth))

	if allowed == 0 {
		if e.orgAllowsOverage(ctx, orgID, limits.PlanTier) {
			e.logger.Info("monthly run cap exceeded with overage enabled",
				"org_id", orgID,
				"plan", limits.DisplayName,
				"limit", limits.MaxRunsPerMonth,
				"current", currentCount,
			)
			e.emitBillingEvent(orgID, "monthly_run_overage", string(limits.PlanTier))
			recordBillingOverageRun(ctx, "monthly_runs", string(limits.PlanTier))
			if err := e.markRunOverage(ctx, runID); err != nil {
				e.logger.Warn("failed to mark monthly run overage, failing closed",
					"org_id", orgID,
					"run_id", runID,
					"error", err,
				)
				return serviceDegradedLimitError()
			}
			return nil
		}

		e.recordRejection(ctx, "monthly_run_limit", limits.PlanTier)
		if err := e.PauseJobsForQuotaExceeded(ctx, orgID); err != nil {
			e.logger.Warn("failed to pause jobs after monthly run cap reached", "org_id", orgID, "error", err)
		}
		message := fmt.Sprintf("Your %s plan allows %d runs per month. You've used %d. Upgrade to continue.", limits.DisplayName, limits.MaxRunsPerMonth, currentCount)
		if limits.PlanTier != domain.PlanFree {
			message = fmt.Sprintf("Your %s plan allows %d included runs per month and overage is disabled. You've used %d. Enable overage or upgrade to continue.", limits.DisplayName, limits.MaxRunsPerMonth, currentCount)
		}
		return &LimitError{
			Code:         "plan_cap_reached",
			Message:      message,
			CurrentUsage: currentCount,
			Limit:        int64(limits.MaxRunsPerMonth),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	return nil
}

func (e *Enforcer) orgAllowsOverage(ctx context.Context, orgID string, tier domain.PlanTier) bool {
	if e.store == nil {
		return tier != domain.PlanFree
	}
	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil || sub == nil {
		return tier != domain.PlanFree
	}
	return !sub.OverageDisabled
}

func (e *Enforcer) markRunOverage(ctx context.Context, runID string) error {
	if runID == "" || e.rdb == nil {
		return nil
	}
	if err := e.rdb.Set(ctx, runOverageKey(runID), "1", time.Duration(monthlyRunCounterTTLSecs)*time.Second).Err(); err != nil {
		return fmt.Errorf("mark run overage: %w", err)
	}
	return nil
}

func (e *Enforcer) IsRunOverage(ctx context.Context, runID string) bool {
	if runID == "" || e.rdb == nil {
		return false
	}
	ok, err := e.rdb.Exists(ctx, runOverageKey(runID)).Result()
	if err != nil {
		e.logger.Warn("failed to read run overage marker", "run_id", runID, "error", err)
		return false
	}
	return ok > 0
}

func runOverageKey(runID string) string {
	return "billing:run_overage:" + runID
}

// GetMonthlyRunCount returns the current monthly run count for an org from Redis.
// Returns 0 on any error or if no key exists.
func (e *Enforcer) GetMonthlyRunCount(ctx context.Context, orgID string) (int64, error) {
	if orgID == "" || e.rdb == nil {
		return 0, nil
	}
	key := monthlyRunKey(orgID, time.Now())
	count, err := e.rdb.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("getting monthly run count: %w", err)
	}
	return count, nil
}

// PauseJobsForQuotaExceeded pauses all HTTP-mode jobs for an org with reason
// "quota_exceeded". Called when the org exceeds their monthly cap on free tier.
func (e *Enforcer) PauseJobsForQuotaExceeded(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	pausedIDs, err := e.store.PauseHTTPJobsByOrg(ctx, orgID, "quota_exceeded")
	if err != nil {
		return fmt.Errorf("pausing jobs for quota exceeded (org=%s): %w", orgID, err)
	}
	e.logger.Info("paused jobs for quota exceeded",
		"org_id", orgID,
		"jobs_paused", len(pausedIDs),
	)
	e.emitBillingEvent(orgID, "jobs_paused_quota_exceeded", "")
	e.dispatchScheduleSuspended(ctx, orgID, pausedIDs, "quota_exceeded")
	return nil
}

// dispatchScheduleSuspended emits one schedule.suspended webhook event per
// paused job. Failures are logged but do not propagate so callers can return
// success from the underlying state transition.
func (e *Enforcer) dispatchScheduleSuspended(ctx context.Context, orgID string, jobIDs []string, reason string) {
	if e.billingDispatcher == nil || len(jobIDs) == 0 || orgID == "" {
		return
	}
	tier := e.tierForOrg(ctx, orgID)
	for _, jobID := range jobIDs {
		detail := map[string]any{
			"schedule_id": jobID,
			"reason":      reason,
		}
		if err := DispatchBillingWebhook(ctx, e.billingDispatcher, orgID, tier, domain.WebhookEventScheduleSuspended, detail); err != nil {
			e.logger.Warn("dispatch schedule.suspended failed",
				"org_id", orgID,
				"job_id", jobID,
				"reason", reason,
				"error", err,
			)
		}
	}
}

// tierForOrg returns the plan tier for an org or PlanFree if it can't be resolved.
func (e *Enforcer) tierForOrg(ctx context.Context, orgID string) domain.PlanTier {
	if e.store == nil {
		return domain.PlanFree
	}
	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil || sub == nil {
		return domain.PlanFree
	}
	return domain.PlanTier(sub.PlanTier)
}

// ResumeJobsAfterQuotaReset unpauses jobs that were paused due to quota exceeded.
// Call this at the start of a new billing period.
func (e *Enforcer) ResumeJobsAfterQuotaReset(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	resumed, err := e.store.UnpauseJobsByPauseReason(ctx, orgID, "quota_exceeded")
	if err != nil {
		return fmt.Errorf("resuming jobs after quota reset (org=%s): %w", orgID, err)
	}
	e.logger.Info("resumed jobs after quota reset",
		"org_id", orgID,
		"jobs_resumed", resumed,
	)
	e.emitBillingEvent(orgID, "jobs_resumed_quota_reset", "")
	return nil
}

// Check80PercentMonthlyWarning returns true when the org has used 80% or more of
// its monthly run cap and a warning email has not yet been sent this period.
// Returns (false, nil) for unlimited plans.
func (e *Enforcer) Check80PercentMonthlyWarning(ctx context.Context, orgID string) (bool, error) {
	if orgID == "" || e.rdb == nil {
		return false, nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("getting org plan limits for monthly warning: %w", err)
	}

	if limits.MaxRunsPerMonth == -1 {
		return false, nil
	}

	count, err := e.GetMonthlyRunCount(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("getting monthly run count for warning check: %w", err)
	}

	threshold := int64(float64(limits.MaxRunsPerMonth) * 0.8)
	if count < threshold {
		return false, nil
	}

	// Only send once per billing period — use a Redis key to track.
	periodKey := fmt.Sprintf("strait:billing_email:%s:monthly_80pct:%s",
		orgID, time.Now().UTC().Format("2006-01"))
	set, err := e.rdb.SetNX(ctx, periodKey, "1", 32*24*time.Hour).Result()
	if err != nil {
		// Fail open: surface the warning if we can't track dedup.
		return true, err
	}
	return set, nil
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
	if orgID == "" {
		return nil
	}
	if e.rdb == nil {
		if e.requireRedis {
			e.logger.Warn("concurrent run limit unavailable: Redis client not configured", "org_id", orgID)
			return serviceDegradedLimitError()
		}
		return nil
	}

	if err := e.checkPaymentStatus(ctx, orgID); err != nil {
		return err
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for concurrent check", "org_id", orgID, "error", err)
		return e.failClosedPlanLimitLookup(ctx, orgID, "concurrent_limit", err)
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
		return serviceDegradedLimitError()
	}

	if result == -1 {
		// Script returned -1 meaning at/over limit (DECR already called).
		currentCount, _ := e.rdb.Get(ctx, key).Int64()
		recordBillingQuotaUsage(ctx, "concurrent_runs", string(limits.PlanTier), quotaUsageRatio(currentCount, int64(limits.MaxConcurrentRuns)))
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
	recordBillingQuotaUsage(ctx, "concurrent_runs", string(limits.PlanTier), quotaUsageRatio(result, int64(limits.MaxConcurrentRuns)))

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

// CheckMaxDispatchPriority checks whether requestedPriority is within the
// launch platform cap resolved from the org's current plan. Priority is not a
// paid launch entitlement; every active plan uses the same bounded range.
//
// MaxDispatchPriority semantics:
//   - -1  unlimited
//   - 0   only the default priority
//   - N>0 priorities 0..N are allowed
//
// projectID is used to resolve the org. Returns a *LimitError when the
// requested priority exceeds the cap; nil on success.
func (e *Enforcer) CheckMaxDispatchPriority(ctx context.Context, projectID string, requestedPriority int) error {
	if e == nil || projectID == "" || requestedPriority <= 0 {
		return nil // priority 0 is always valid
	}

	orgID, err := e.store.GetProjectOrgID(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to resolve org for dispatch priority check",
			"project_id", projectID, "error", err)
		// Fail closed: a lookup failure must not grant elevated priority.
		return &LimitError{
			Code:    "dispatch_priority_exceeded",
			Message: fmt.Sprintf("could not verify plan limits: %v", err),
		}
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for dispatch priority check",
			"org_id", orgID, "error", err)
		// Fail closed: a lookup failure must not grant elevated priority.
		return &LimitError{
			Code: "dispatch_priority_exceeded",
			Message: fmt.Sprintf(
				"could not verify plan limits: %v. Requested priority %d was rejected.",
				err, requestedPriority,
			),
		}
	}

	if limits.MaxDispatchPriority == -1 {
		return nil // unlimited
	}

	if requestedPriority > limits.MaxDispatchPriority {
		e.recordRejection(ctx, "dispatch_priority", limits.PlanTier)
		return &LimitError{
			Code: "dispatch_priority_exceeded",
			Message: fmt.Sprintf(
				"Dispatch priority must be at most %d. Requested: %d.",
				limits.MaxDispatchPriority, requestedPriority,
			),
			CurrentUsage: int64(requestedPriority),
			Limit:        int64(limits.MaxDispatchPriority),
			Plan:         string(limits.PlanTier),
		}
	}

	return nil
}

// CheckProjectLimit checks if org can create another project.
func (e *Enforcer) CheckProjectLimit(ctx context.Context, orgID string) error {
	if e == nil || orgID == "" {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for project check", "org_id", orgID, "error", err)
		return e.failClosedPlanLimitLookup(ctx, orgID, "project_limit", err)
	}

	if limits.MaxProjectsPerOrg == -1 {
		return nil
	}

	count, err := e.store.CountProjectsByOrg(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to count org projects for project limit check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}
	e.resetFailOpen(orgID, "project_limit")

	if count >= limits.MaxProjectsPerOrg {
		recordBillingQuotaUsage(ctx, "projects", string(limits.PlanTier), quotaUsageRatio(int64(count), int64(limits.MaxProjectsPerOrg)))
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
	recordBillingQuotaUsage(ctx, "projects", string(limits.PlanTier), quotaUsageRatio(int64(count), int64(limits.MaxProjectsPerOrg)))

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
			return e.checkFreeTierIncludedCredit(ctx, orgID, nil)
		}
		e.logger.Warn("failed to get org subscription for spending check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	if limits.PlanTier == domain.PlanFree {
		return e.checkFreeTierIncludedCredit(ctx, orgID, sub)
	}

	if sub.SpendingLimitMicrousd == -1 {
		return nil // no limit set
	}

	// The previous revision wrapped the spend check in a Redis lock with a
	// sleep-retry loop (up to 600ms) intended to "reduce the TOCTOU window".
	// In practice the lock provided zero correctness benefit: SumOrgPeriodSpend
	// is a stateless read, no cached value is written under the lock, and the
	// caller already accepts (by comment) that the unserialized path is safe.
	// What it did do was block the caller's goroutine for up to 600ms under
	// concurrent spend checks for the same org — which is exactly the regime
	// where we most want this code to be fast. Fail-open on contention by
	// doing the work unserialized; the in-flight call's result is identical.
	periodStart, _ := usagePeriodWindow(time.Now().UTC(), limits.PlanTier, sub)
	periodSpend, err := e.store.SumOrgPeriodSpend(ctx, orgID, periodStart)
	if err != nil {
		e.logger.Warn("failed to sum org period spend", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	overageSpend := computeOverageSpend(periodSpend, 0)
	recordBillingQuotaUsage(ctx, "spend", string(limits.PlanTier), quotaUsageRatio(overageSpend, sub.SpendingLimitMicrousd))

	// Send spending alerts (async with 24h cooldown per org).
	if sub.SpendingLimitMicrousd > 0 {
		spendPct := float64(overageSpend) / float64(sub.SpendingLimitMicrousd)
		if e.billingEmails != nil && spendPct >= 0.8 && spendPct < 1.0 && e.shouldSendBillingEmail(ctx, orgID, "spending_80pct") {
			adminEmails, _ := e.store.ListOrgAdminEmails(ctx, orgID)
			e.bgWG.Go(func() {
				emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				e.billingEmails.SendSpendingLimitWarning(emailCtx, adminEmails, sub.PlanTier,
					fmt.Sprintf("$%.2f", float64(periodSpend)/1e6),
					fmt.Sprintf("$%.2f", float64(sub.SpendingLimitMicrousd)/1e6),
					fmt.Sprintf("%.0f%%", spendPct*100))
			})
		}
		if spendPct >= 0.8 && spendPct < 1.0 {
			e.tryDispatchCapEvent(ctx, orgID, limits.PlanTier,
				BillingCapEventWarning,
				domain.WebhookEventBillingCapWarning,
				map[string]any{
					"spend_pct":            spendPct,
					"period_spend":         periodSpend,
					"spending_limit":       sub.SpendingLimitMicrousd,
					"current_period_start": sub.CurrentPeriodStart,
				})
		}
	}

	// Send overage alert when org first enters overage.
	if overageSpend > 0 && e.billingEmails != nil && e.shouldSendBillingEmail(ctx, orgID, "overage_entered") {
		adminEmails, _ := e.store.ListOrgAdminEmails(ctx, orgID)
		e.bgWG.Go(func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e.billingEmails.SendOverageAlert(emailCtx, adminEmails, sub.PlanTier,
				fmt.Sprintf("$%.2f", float64(overageSpend)/1e6),
				"$0.00")
		})
	}

	if isOverageLimitReached(sub.SpendingLimitMicrousd, overageSpend) {
		e.recordRejection(ctx, "spending_limit", limits.PlanTier)
		e.emitBillingEvent(orgID, "spending_limit_hit", sub.PlanTier)

		capDetail := map[string]any{
			"period_spend":         periodSpend,
			"overage_spend":        overageSpend,
			"spending_limit":       sub.SpendingLimitMicrousd,
			"limit_action":         sub.LimitAction,
			"current_period_start": sub.CurrentPeriodStart,
		}
		e.tryDispatchCapEvent(ctx, orgID, limits.PlanTier,
			BillingCapEventReached,
			domain.WebhookEventBillingCapReached, capDetail)
		switch sub.LimitAction {
		case "disable":
			e.tryDispatchCapEvent(ctx, orgID, limits.PlanTier,
				BillingCapEventDisabled,
				domain.WebhookEventBillingCapDisabled, capDetail)
		case "block", "reject":
			e.tryDispatchCapEvent(ctx, orgID, limits.PlanTier,
				BillingCapEventOverageDisabled,
				domain.WebhookEventBillingOverageDisabled, capDetail)
		}

		if spendingLimitActionBlocks(sub.LimitAction) {
			if pauseErr := e.PauseJobsForQuotaExceeded(ctx, orgID); pauseErr != nil {
				e.logger.Warn("failed to pause jobs after spending limit reached", "org_id", orgID, "error", pauseErr)
			}
			return &LimitError{
				Code:         "spending_limit_reached",
				Message:      fmt.Sprintf("Your monthly spending limit of $%.2f has been reached.", float64(sub.SpendingLimitMicrousd)/1000000),
				CurrentUsage: overageSpend,
				Limit:        sub.SpendingLimitMicrousd,
				Plan:         sub.PlanTier,
				UpgradeURL:   "/settings/billing",
			}
		}
	}

	return nil
}

func spendingLimitActionBlocks(action string) bool {
	switch action {
	case "block", "reject", "disable":
		return true
	default:
		return false
	}
}

func (e *Enforcer) checkFreeTierIncludedCredit(ctx context.Context, orgID string, sub *OrgSubscription) error {
	periodStart, _ := usagePeriodWindow(time.Now().UTC(), domain.PlanFree, sub)
	periodSpend, err := e.store.SumOrgPeriodSpend(ctx, orgID, periodStart)
	if err != nil {
		e.logger.Warn("failed to sum free-tier period spend", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}

	// Free tier has no included compute credit; any spend is overage.
	overageSpend := computeOverageSpend(periodSpend, 0)
	if !isOverageLimitReached(0, overageSpend) {
		return nil
	}

	e.recordRejection(ctx, "spending_limit", domain.PlanFree)
	return &LimitError{
		Code:         "spending_limit_reached",
		Message:      "Your free plan monthly compute budget has been reached.",
		CurrentUsage: periodSpend,
		Limit:        0,
		Plan:         string(domain.PlanFree),
		UpgradeURL:   "/upgrade",
	}
}

// CheckProjectBudgetLimit hard-rejects dispatch when a project has crossed its
// monthly budget AND budget_action is "block". When budget_action is "notify"
// (the default for unset rows), the call is a no-op — alerting is handled
// separately. When no quota row exists, the call is a no-op.
//
// projectID is required; an empty value is treated as a no-op so callers do not
// have to nil-guard at every dispatch site.
func (e *Enforcer) CheckProjectBudgetLimit(ctx context.Context, projectID string) error {
	if e == nil || projectID == "" {
		return nil
	}

	budget, action, err := e.store.GetProjectBudget(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to read project budget", "project_id", projectID, "error", err)
		return serviceDegradedLimitError()
	}

	// budget_action="notify" or unset means the budget is informational only.
	// budget < 0 indicates "no budget configured" via the GetProjectBudget
	// sentinel and must always pass.
	if !projectBudgetActionBlocks(action) || budget < 0 {
		return nil
	}

	// Resolve org so we can use the same period window the spending check uses.
	// A zero-value OrgSubscription gives us the canonical month-boundary
	// fallback inside usagePeriodWindow.
	orgID, err := e.store.GetProjectOrgID(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to resolve org for project budget check",
			"project_id", projectID, "error", err)
		return serviceDegradedLimitError()
	}

	var sub *OrgSubscription
	if orgID != "" {
		sub, _ = e.store.GetOrgSubscription(ctx, orgID)
	}
	tier := domain.PlanFree
	if sub != nil {
		tier = domain.PlanTier(sub.PlanTier)
	}
	periodStart, _ := usagePeriodWindow(time.Now().UTC(), tier, sub)

	spend, err := e.store.GetProjectPeriodSpend(ctx, projectID, periodStart)
	if err != nil {
		e.logger.Warn("failed to read project period spend",
			"project_id", projectID, "error", err)
		return serviceDegradedLimitError()
	}

	if !isOverageLimitReached(budget, spend) {
		return nil
	}

	planLabel := string(tier)
	if sub != nil && sub.PlanTier != "" {
		planLabel = sub.PlanTier
	}
	e.recordRejection(ctx, "project_budget", tier)
	return &LimitError{
		Code: "project_budget_reached",
		Message: fmt.Sprintf(
			"This project has reached its monthly budget of $%.2f.",
			float64(budget)/1e6,
		),
		CurrentUsage: spend,
		Limit:        budget,
		Plan:         planLabel,
		UpgradeURL:   "/settings/billing",
	}
}

func projectBudgetActionBlocks(action string) bool {
	switch action {
	case "block", "reject":
		return true
	default:
		return false
	}
}

func workerConnectionReservationsKey(orgID string) string {
	return "strait:worker_connections:" + orgID
}

var workerConnectionReserveScript = redis.NewScript(`
local key = KEYS[1]
local reservation = ARGV[1]
local now_ms = tonumber(ARGV[2])
local expires_ms = tonumber(ARGV[3])
local limit = tonumber(ARGV[4])
local ttl_seconds = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms)
local count = redis.call('ZCARD', key)
if limit ~= -1 and count >= limit then
  return {0, count}
end
redis.call('ZADD', key, expires_ms, reservation)
redis.call('EXPIRE', key, ttl_seconds)
return {1, count + 1}
`)

const defaultWorkerConnectionLease = 90 * time.Second

func normalizeWorkerConnectionLease(lease time.Duration) time.Duration {
	if lease <= 0 {
		return defaultWorkerConnectionLease
	}
	if lease < 10*time.Second {
		return 10 * time.Second
	}
	return lease
}

// ReserveWorkerConnection creates a distributed, expiring worker-connection
// reservation for orgID. The returned release function must be called when the
// stream exits. Heartbeats should call RenewWorkerConnection before lease
// expiry so long-lived streams remain counted across API replicas.
func (e *Enforcer) ReserveWorkerConnection(ctx context.Context, orgID, reservationID string, lease time.Duration) (func(), error) {
	releaseNoop := func() {}
	if e == nil || orgID == "" || reservationID == "" {
		return releaseNoop, nil
	}
	if e.rdb == nil {
		e.logger.Warn("worker connection reservation unavailable: Redis client not configured", "org_id", orgID)
		return releaseNoop, serviceDegradedLimitError()
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for worker connection reservation",
			"org_id", orgID, "error", err)
		return releaseNoop, e.failClosedPlanLimitLookup(ctx, orgID, "worker_connections", err)
	}
	if limits.WorkerConnections == -1 {
		return releaseNoop, nil
	}

	lease = normalizeWorkerConnectionLease(lease)
	now := time.Now().UTC()
	ttl := max(int((lease * 2).Seconds()), 1)
	result, err := workerConnectionReserveScript.Run(ctx, e.rdb, []string{workerConnectionReservationsKey(orgID)},
		reservationID,
		now.UnixMilli(),
		now.Add(lease).UnixMilli(),
		limits.WorkerConnections,
		ttl,
	).Result()
	if err != nil {
		e.logger.Warn("failed to reserve worker connection", "org_id", orgID, "error", err)
		return releaseNoop, serviceDegradedLimitError()
	}

	vals, ok := result.([]any)
	if !ok || len(vals) < 2 {
		e.logger.Warn("unexpected worker connection reservation result", "org_id", orgID)
		return releaseNoop, serviceDegradedLimitError()
	}
	allowed, _ := vals[0].(int64)
	current, _ := vals[1].(int64)
	if allowed == 0 {
		e.recordRejection(ctx, "worker_connections", limits.PlanTier)
		return releaseNoop, &LimitError{
			Code: "worker_connections_reached",
			Message: fmt.Sprintf(
				"Your %s plan allows %d concurrent worker connections per organization. Active: %d. Upgrade to add more.",
				limits.DisplayName, limits.WorkerConnections, current,
			),
			CurrentUsage: current,
			Limit:        int64(limits.WorkerConnections),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
	}

	released := sync.Once{}
	return func() {
		released.Do(func() {
			if err := e.rdb.ZRem(context.Background(), workerConnectionReservationsKey(orgID), reservationID).Err(); err != nil {
				e.logger.Warn("failed to release worker connection reservation",
					"org_id", orgID, "error", err)
			}
		})
	}, nil
}

// RenewWorkerConnection extends an existing distributed worker-connection
// reservation. Missing reservations are left missing; the next reconnect will
// create a fresh reservation and enforce the cap again.
func (e *Enforcer) RenewWorkerConnection(ctx context.Context, orgID, reservationID string, lease time.Duration) error {
	if e == nil || orgID == "" || reservationID == "" || e.rdb == nil {
		return nil
	}
	lease = normalizeWorkerConnectionLease(lease)
	expiresAt := time.Now().UTC().Add(lease).UnixMilli()
	ttl := lease * 2
	if err := e.rdb.ZAddXX(ctx, workerConnectionReservationsKey(orgID), redis.Z{
		Score:  float64(expiresAt),
		Member: reservationID,
	}).Err(); err != nil {
		return fmt.Errorf("renew worker connection reservation: %w", err)
	}
	if err := e.rdb.Expire(ctx, workerConnectionReservationsKey(orgID), ttl).Err(); err != nil {
		return fmt.Errorf("renew worker connection reservation ttl: %w", err)
	}
	return nil
}

// CheckWorkerConnectionLimit hard-rejects new gRPC worker registrations once an
// org has hit its plan-tier WorkerConnections cap. Existing connections are
// not evicted; this is a connect-time gate only.
//
// orgID may be empty; in that case the call is a no-op (we cannot enforce a
// per-org cap without knowing which org). currentActive is the registry-local
// count of already-registered workers for the same org. Callers should obtain
// it under the registry lock immediately before invoking this check to keep
// the TOCTOU window minimal — but the check itself is best-effort: a stricter
// global cap would require a distributed counter and is not warranted given
// connections are infrequent.
func (e *Enforcer) CheckWorkerConnectionLimit(ctx context.Context, orgID string, currentActive int) error {
	if e == nil || orgID == "" {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Error("failed to get org plan limits for worker connection check",
			"org_id", orgID, "error", err)
		return fmt.Errorf("resolve worker connection plan limit: %w", err)
	}

	if limits.WorkerConnections == -1 {
		return nil // unlimited
	}

	if currentActive >= limits.WorkerConnections {
		e.recordRejection(ctx, "worker_connections", limits.PlanTier)
		return &LimitError{
			Code: "worker_connections_reached",
			Message: fmt.Sprintf(
				"Your %s plan allows %d concurrent worker connections per organization. Active: %d. Upgrade to add more.",
				limits.DisplayName, limits.WorkerConnections, currentActive,
			),
			CurrentUsage: int64(currentActive),
			Limit:        int64(limits.WorkerConnections),
			Plan:         string(limits.PlanTier),
			UpgradeURL:   "/upgrade",
		}
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

// GetStripeCustomerID returns the Stripe customer ID for an org's subscription.
// Returns empty string if the org has no subscription or no Stripe customer.
func (e *Enforcer) GetStripeCustomerID(ctx context.Context, orgID string) (string, error) {
	sub, err := e.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		return "", err
	}
	if sub.StripeCustomerID == nil || *sub.StripeCustomerID == "" {
		return "", nil
	}
	return *sub.StripeCustomerID, nil
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
// for total Redis failure. Runs can last many hours, so shorter
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

func (e *Enforcer) recordRejection(ctx context.Context, reason string, planTier domain.PlanTier) {
	addBillingSentryBreadcrumb(ctx, "limit_rejection", "billing limit rejected", map[string]any{
		"reason":    reason,
		"plan_tier": string(planTier),
	})
	recordBillingLimitRejection(ctx, reason, string(planTier))
}

func (e *Enforcer) recordFailOpen(ctx context.Context, checkType, errorType string) {
	addBillingSentryBreadcrumb(ctx, "fail_open", "billing enforcement failed open", map[string]any{
		"check_type": checkType,
		"error_type": errorType,
	})
	recordBillingFailOpen(ctx, checkType, errorType)
}

func quotaUsageRatio(current, limit int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(current) / float64(limit)
}

func addBillingSentryBreadcrumb(ctx context.Context, operation, message string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["operation"] = operation
	telemetry.AddSentryBreadcrumb(ctx, "billing."+operation, message, data)
}

func (e *Enforcer) applyBillingSentryScope(scope *sentry.Scope, orgID, operation string) {
	mode, region, version := "", "", ""
	if e != nil {
		mode = e.sentryMode
		region = e.sentryRegion
		version = e.sentryVersion
	}
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemBilling,
		Mode:      mode,
		Region:    region,
		Version:   version,
	})
	telemetry.SetSentryTag(scope, telemetry.TagOrgID, orgID)
	telemetry.SetSentryTag(scope, telemetry.TagOperation, operation)
}

// CheckMemberLimit checks if the org can add another member.
func (e *Enforcer) CheckMemberLimit(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}

	limits, err := e.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to get org plan limits for member check", "org_id", orgID, "error", err)
		return e.failClosedPlanLimitLookup(ctx, orgID, "member_limit", err)
	}

	if limits.MaxMembersPerOrg == -1 {
		return nil
	}

	count, err := e.store.CountMembersByOrg(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to count org members for member limit check", "org_id", orgID, "error", err)
		return serviceDegradedLimitError()
	}
	e.resetFailOpen(orgID, "member_limit")

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
		e.logger.Warn("failed to count user organizations for org creation limit check", "user_id", userID, "error", err)
		return serviceDegradedLimitError()
	}
	e.resetFailOpen(userID, "org_creation_limit")

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

// suspendedCacheEntry stores a cached suspension check result.
type suspendedCacheEntry struct {
	suspended bool
	checkedAt time.Time
}

const suspendedCacheTTL = 30 * time.Second

// CheckProjectSuspended checks if a project is suspended due to plan downgrade.
// Returns ErrProjectSuspended if the project has been soft-locked.
// Results are cached for 30 seconds to avoid a DB round-trip on every dispatch.
func (e *Enforcer) CheckProjectSuspended(ctx context.Context, projectID string) error {
	if projectID == "" {
		return nil
	}

	// Check in-memory cache first.
	if cached, ok := e.suspendedCache.Load(projectID); ok {
		entry := cached.(*suspendedCacheEntry)
		if time.Since(entry.checkedAt) < suspendedCacheTTL {
			if entry.suspended {
				return &LimitError{
					Code:       "project_suspended",
					Message:    "This project is suspended due to a plan downgrade. Upgrade your plan or remove excess projects to restore access.",
					UpgradeURL: "/upgrade",
				}
			}
			return nil
		}
	}

	suspended, err := e.store.IsProjectSuspended(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to check project suspended status",
			"project_id", projectID, "error", err)
		return serviceDegradedLimitError()
	}

	// Cache the result.
	e.suspendedCache.Store(projectID, &suspendedCacheEntry{
		suspended: suspended,
		checkedAt: time.Now(),
	})

	if suspended {
		return &LimitError{
			Code:       "project_suspended",
			Message:    "This project is suspended due to a plan downgrade. Upgrade your plan or remove excess projects to restore access.",
			UpgradeURL: "/upgrade",
		}
	}

	return nil
}

// InvalidateProjectSuspendedCache clears the suspended status cache for a project.
// Call this after changing a project's suspended status.
func (e *Enforcer) InvalidateProjectSuspendedCache(projectID string) {
	e.suspendedCache.Delete(projectID)
}

// FlushSuspendedCacheForOrg removes cached suspension status for all given project IDs.
func (e *Enforcer) FlushSuspendedCacheForOrg(projectIDs []string) {
	for _, pid := range projectIDs {
		e.suspendedCache.Delete(pid)
	}
}

// SuspendExcessProjects suspends projects that exceed the plan limit for an org,
// keeping the oldest projects active. Returns the number of projects suspended.
func (e *Enforcer) SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error) {
	if maxProjects == -1 {
		return 0, nil // unlimited
	}
	return e.store.SuspendExcessProjects(ctx, orgID, maxProjects)
}

// EnsureOrgSubscription delegates to the underlying store to lazily
// initialize a free-tier subscription row for the given org.
func (e *Enforcer) EnsureOrgSubscription(ctx context.Context, orgID string) error {
	return e.store.EnsureOrgSubscription(ctx, orgID)
}

// usagePeriodWindow returns the billing period start and end times for an org.
// For free-tier or missing subscriptions the calendar month is used; for paid
// plans the Stripe billing period anchors are preferred when available.
func usagePeriodWindow(now time.Time, tier domain.PlanTier, sub *OrgSubscription) (time.Time, time.Time) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)

	if tier == domain.PlanFree || sub == nil {
		return monthStart, monthEnd
	}

	start := monthStart
	end := monthEnd
	if sub.CurrentPeriodStart != nil {
		start = *sub.CurrentPeriodStart
	}
	if sub.CurrentPeriodEnd != nil {
		end = *sub.CurrentPeriodEnd
	}
	return start, end
}

// computeOverageSpend returns the portion of periodSpend that exceeds includedCredit.
// Returns 0 if spend is within the included credit.
func computeOverageSpend(periodSpend, includedCredit int64) int64 {
	return max(periodSpend-includedCredit, 0)
}

// isOverageLimitReached reports whether the overage spend has reached the configured limit.
// A limitMicrousd of -1 means uncapped; 0 means any overage triggers the limit.
func isOverageLimitReached(limitMicrousd, overageSpendMicrousd int64) bool {
	switch {
	case limitMicrousd < 0:
		return false
	case limitMicrousd == 0:
		return overageSpendMicrousd > 0
	default:
		return overageSpendMicrousd >= limitMicrousd
	}
}

// MaxSpendingLimit returns the maximum allowed spending limit for a plan tier.
func MaxSpendingLimit(tier domain.PlanTier) int64 {
	switch tier {
	case domain.PlanFree:
		return MaxSpendingFree
	case domain.PlanStarter:
		return MaxSpendingStarter
	case domain.PlanPro:
		return MaxSpendingPro
	case domain.PlanScale:
		return MaxSpendingScale
	case domain.PlanBusiness:
		return MaxSpendingBusiness
	case domain.PlanEnterprise:
		return -1 // custom
	default:
		return 0
	}
}

// SpendingLimitResponse is the API response for spending limit queries.
type SpendingLimitResponse struct {
	OrgID            string  `json:"org_id"`
	PlanTier         string  `json:"plan_tier"`
	OverageEnabled   bool    `json:"overage_enabled"`
	SpendingLimitUsd float64 `json:"spending_limit_usd"`
	LimitAction      string  `json:"limit_action"`
	CurrentSpendUsd  float64 `json:"current_spend_usd"`
	OverageSpendUsd  float64 `json:"overage_spend_usd"`
	IsHardCapped     bool    `json:"is_hard_capped"`
}

// ApplySubscriptionAddOns is kept as an inert compatibility step for legacy
// organization_subscriptions.add_ons rows. Launch add-ons are represented by
// organization_addons and applied by EffectiveLimits.
func ApplySubscriptionAddOns(base OrgPlanLimits, _ SubscriptionAddOns) OrgPlanLimits {
	return base
}
