//go:build loadtest

package loadtest

import (
	"testing"
)

func TestAnalytics_GetPerformance(t *testing.T) {
	mustClean(t)
	projectID := "proj-perf-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 50)

	tgt := newProjectTargeter("GET", "/v1/analytics/performance", projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "analytics-performance", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "analytics-performance", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "analytics-performance", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestAnalytics_GetPerformanceWithPeriod(t *testing.T) {
	mustClean(t)
	projectID := "proj-perf-period-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 50)

	tgt := newProjectTargeter("GET", "/v1/analytics/performance?period_hours=72", projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "analytics-performance-period", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "analytics-performance-period", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "analytics-performance-period", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
