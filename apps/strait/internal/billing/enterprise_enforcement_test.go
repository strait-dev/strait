package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestEnforcement_Enterprise_NoConcurrentRunLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxConcurrentRuns)

}

func TestEnforcement_Enterprise_NoDailyRunLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxRunsPerDay)

}

func TestEnforcement_Enterprise_NoProjectLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxProjectsPerOrg)

}

func TestEnforcement_Enterprise_NoMemberLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxMembersPerOrg)

}

func TestEnforcement_Enterprise_HTTPModeAllowed(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.True(t, e.
		AllowsHTTPMode,
	)

}

func TestEnforcement_Enterprise_NoSpendingLimitCap(t *testing.T) {
	t.Parallel()
	// MaxSpendingLimit for enterprise should be -1 (uncapped).
	maxSpend := MaxSpendingLimit(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		maxSpend)

}

func TestEnforcement_Enterprise_CannotBypassViaInvalidTierString(t *testing.T) {
	t.Parallel()
	// Attempting to use "ENTERPRISE" (uppercase) should fall back to free tier.
	limits := GetPlanLimits(domain.PlanTier("ENTERPRISE"))
	assert.Equal(t, domain.
		PlanFree, limits.PlanTier,
	)
	assert.False(t, limits.
		HasSSO)

}

func TestEnforcement_Enterprise_PlanDowngradeFromEnterprise(t *testing.T) {
	t.Parallel()
	// Downgrade from enterprise to any lower tier should be detected.
	for _, target := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
		assert.True(t, IsDowngrade(domain.PlanEnterprise,

			target))

	}
}

func TestEnforcement_Enterprise_CaseSensitiveTierLookup(t *testing.T) {
	t.Parallel()
	// Only lowercase "enterprise" is valid.
	valid := domain.PlanTier("enterprise")
	assert.True(t, valid.
		IsValid())

	invalid := domain.PlanTier("Enterprise")
	assert.False(t, invalid.
		IsValid())

}

func TestEnforcement_Enterprise_UnlimitedScheduledJobs(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxScheduledJobs)

}

func TestEnforcement_Enterprise_UnlimitedWebhookEndpoints(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxWebhookEndpoints)

}

func TestEnforcement_Enterprise_CustomWebhookEventLevel(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.Equal(t, "all_custom",

		e.WebhookEventLevel,
	)

}
