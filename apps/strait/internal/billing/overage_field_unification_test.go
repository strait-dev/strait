package billing

import (
	"reflect"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOveragePerKMicrousd_SingleField guards the invariant that OrgPlanLimits
// exposes one canonical per-1K overage field. The duplicate
// OveragePerKRunsMicrousd has been removed; any reintroduction must justify the
// alias and update this test.
func TestOveragePerKMicrousd_SingleField(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeFor[OrgPlanLimits]()
	if _, ok := rt.FieldByName("OveragePerKRunsMicrousd"); ok {
		require.Failf(t, "test failure",

			"OveragePerKRunsMicrousd should be removed; use OveragePerKMicrousd")
	}
	if _, ok := rt.FieldByName("OveragePerKMicrousd"); !ok {
		require.Failf(t, "test failure",

			"OveragePerKMicrousd is the canonical field; missing")
	}
}

// TestOveragePerKMicrousd_AllTiersPopulated locks the per-tier rates against
// the Notion canonical table so a future field-collapse cannot leave a tier
// with the zero value silently.
func TestOveragePerKMicrousd_AllTiersPopulated(t *testing.T) {
	t.Parallel()

	want := map[domain.PlanTier]int64{
		domain.PlanFree:       FreeOveragePerKMicrousd,
		domain.PlanStarter:    StarterOveragePerKMicrousd,
		domain.PlanPro:        ProOveragePerKMicrousd,
		domain.PlanScale:      ScaleOveragePerKMicrousd,
		domain.PlanBusiness:   BusinessOveragePerKMicrousd,
		domain.PlanEnterprise: EnterpriseOveragePerKMicrousd,
	}
	for tier, expect := range want {
		got := GetPlanLimits(tier).OveragePerKMicrousd
		assert.Equal(t, expect,
			got)

	}
}
