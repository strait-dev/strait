//go:build integration

package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"

	"strait/internal/compute"
	"strait/internal/domain"
	"strait/internal/store"
)

// --- Budget Reservation Integration Tests ---

func TestReserveBudget_WithinLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	mustCreateProjectQuota(t, ctx, q, projectID, 100000) // $0.10 limit

	err := q.ReserveBudget(ctx, projectID, newID(), newID(), "micro", 5000, "UTC", 100000)
	if err != nil {
		t.Fatalf("ReserveBudget() error = %v", err)
	}

	// Verify reservation exists.
	cost, err := q.SumDailyComputeCost(ctx, projectID, "UTC")
	if err != nil {
		t.Fatalf("SumDailyComputeCost() error = %v", err)
	}
	if cost != 5000 {
		t.Errorf("daily cost = %d, want 5000 (reserved)", cost)
	}
}

func TestReserveBudget_ExceedsLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	mustCreateProjectQuota(t, ctx, q, projectID, 10000)

	err := q.ReserveBudget(ctx, projectID, newID(), newID(), "micro", 15000, "UTC", 10000)
	if err == nil {
		t.Fatal("expected ErrBudgetExceeded")
	}
	if err != store.ErrBudgetExceeded {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestReserveBudget_ConcurrentRace(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	jobID := newID()
	limit := int64(10000) // $0.01
	mustCreateProjectQuota(t, ctx, q, projectID, limit)

	// Launch two concurrent reservations of $0.006 each.
	// Only one should succeed (6000+6000 = 12000 > 10000).
	var wg conc.WaitGroup
	var successes int
	var mu sync.Mutex

	for range 2 {
		wg.Go(func() {
			err := q.ReserveBudget(ctx, projectID, newID(), jobID, "micro", 6000, "UTC", limit)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 success from 2 concurrent reservations, got %d", successes)
	}
}

func TestCommitReservation_UpdatesCostAndStatus(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	runID := newID()
	mustCreateProjectQuota(t, ctx, q, projectID, 100000)

	// Reserve.
	err := q.ReserveBudget(ctx, projectID, runID, newID(), "micro", 5000, "UTC", 100000)
	if err != nil {
		t.Fatalf("ReserveBudget() error = %v", err)
	}

	// Commit with actual cost.
	now := time.Now()
	started := now.Add(-10 * time.Second)
	err = q.CommitReservation(ctx, runID, 1700, 10.0, "m-123", &started, &now)
	if err != nil {
		t.Fatalf("CommitReservation() error = %v", err)
	}

	// Verify cost is now actual, not estimated.
	cost, err := q.SumDailyComputeCost(ctx, projectID, "UTC")
	if err != nil {
		t.Fatalf("SumDailyComputeCost() error = %v", err)
	}
	if cost != 1700 {
		t.Errorf("daily cost = %d, want 1700 (committed actual)", cost)
	}
}

func TestReleaseReservation_DeletesRow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	runID := newID()
	mustCreateProjectQuota(t, ctx, q, projectID, 100000)

	// Reserve.
	err := q.ReserveBudget(ctx, projectID, runID, newID(), "micro", 5000, "UTC", 100000)
	if err != nil {
		t.Fatalf("ReserveBudget() error = %v", err)
	}

	// Release.
	err = q.ReleaseReservation(ctx, runID)
	if err != nil {
		t.Fatalf("ReleaseReservation() error = %v", err)
	}

	// Verify cost is 0.
	cost, err := q.SumDailyComputeCost(ctx, projectID, "UTC")
	if err != nil {
		t.Fatalf("SumDailyComputeCost() error = %v", err)
	}
	if cost != 0 {
		t.Errorf("daily cost = %d, want 0 (released)", cost)
	}
}

func TestCleanupStaleReservations_RemovesOld(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	mustCreateProjectQuota(t, ctx, q, projectID, 100000)

	// Insert a reservation manually with old created_at.
	oldID := uuid.Must(uuid.NewV7()).String()
	_, err := testDB.Pool.Exec(ctx,
		`INSERT INTO run_compute_usage (id, run_id, project_id, job_id, machine_preset, cost_microusd, status, created_at)
		 VALUES ($1, $2, $3, $4, 'micro', 5000, 'reserved', NOW() - INTERVAL '3 hours')`,
		oldID, newID(), projectID, newID())
	if err != nil {
		t.Fatalf("insert old reservation: %v", err)
	}

	// Insert a fresh reservation.
	err = q.ReserveBudget(ctx, projectID, newID(), newID(), "micro", 3000, "UTC", 100000)
	if err != nil {
		t.Fatalf("ReserveBudget() error = %v", err)
	}

	// Cleanup with 2h TTL.
	deleted, err := q.CleanupStaleReservations(ctx, 2*time.Hour)
	if err != nil {
		t.Fatalf("CleanupStaleReservations() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only old reservation)", deleted)
	}

	// Fresh reservation should still count.
	cost, err := q.SumDailyComputeCost(ctx, projectID, "UTC")
	if err != nil {
		t.Fatalf("SumDailyComputeCost() error = %v", err)
	}
	if cost != 3000 {
		t.Errorf("daily cost = %d, want 3000 (fresh reservation kept)", cost)
	}
}

func TestReserveBudget_NoQuota_SkipGracefully(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	// Verify reservation works even when there's no project_quotas row.
	// The caller (executor) should skip reservation when quota is nil.
	// This test validates the store method itself with a 0 limit.
	err := q.ReserveBudget(ctx, newID(), newID(), newID(), "micro", 5000, "UTC", 0)
	if err == nil {
		t.Error("expected error when limit is 0 (cost > 0 limit)")
	}
}

// --- Preset Recommendation Integration Tests ---

func TestRecordOOMEvent_FirstTime(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	jobID := newID()
	err := q.RecordOOMEvent(ctx, jobID, "micro")
	if err != nil {
		t.Fatalf("RecordOOMEvent() error = %v", err)
	}

	rec, err := q.GetPresetRecommendation(ctx, jobID)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected recommendation to exist")
	}
	if rec.OOMCount != 1 {
		t.Errorf("oom_count = %d, want 1", rec.OOMCount)
	}
	if rec.CurrentPreset != "micro" {
		t.Errorf("current_preset = %q, want micro", rec.CurrentPreset)
	}
	if rec.RecommendedPreset != "small-1x" {
		t.Errorf("recommended_preset = %q, want small-1x", rec.RecommendedPreset)
	}
}

func TestRecordOOMEvent_Increment(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	jobID := newID()

	// First OOM.
	err := q.RecordOOMEvent(ctx, jobID, "micro")
	if err != nil {
		t.Fatalf("first RecordOOMEvent() error = %v", err)
	}

	// Second OOM (with upgraded preset).
	err = q.RecordOOMEvent(ctx, jobID, "small-1x")
	if err != nil {
		t.Fatalf("second RecordOOMEvent() error = %v", err)
	}

	rec, err := q.GetPresetRecommendation(ctx, jobID)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	if rec.OOMCount != 2 {
		t.Errorf("oom_count = %d, want 2", rec.OOMCount)
	}
	if rec.RecommendedPreset != "small-2x" {
		t.Errorf("recommended_preset = %q, want small-2x", rec.RecommendedPreset)
	}
}

func TestRecordOOMEvent_MaxPreset(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	jobID := newID()
	err := q.RecordOOMEvent(ctx, jobID, "large-2x")
	if err != nil {
		t.Fatalf("RecordOOMEvent() error = %v", err)
	}

	rec, err := q.GetPresetRecommendation(ctx, jobID)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	// At max, recommended stays at max.
	if rec.RecommendedPreset != "large-2x" {
		t.Errorf("recommended_preset = %q, want large-2x (already max)", rec.RecommendedPreset)
	}
}

func TestGetPresetRecommendation_Expired(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	jobID := newID()
	// Insert an expired recommendation.
	_, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_preset_recommendations (id, job_id, current_preset, recommended_preset, oom_count, window_start, expires_at)
		 VALUES ($1, $2, 'micro', 'small-1x', 1, NOW() - INTERVAL '25 hours', NOW() - INTERVAL '1 hour')`,
		uuid.Must(uuid.NewV7()).String(), jobID)
	if err != nil {
		t.Fatalf("insert expired rec: %v", err)
	}

	q := mustStore(t)
	rec, err := q.GetPresetRecommendation(ctx, jobID)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	if rec != nil {
		t.Error("expected nil for expired recommendation")
	}
}

func TestCleanupExpiredRecommendations(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	jobID1 := newID()
	jobID2 := newID()

	// Insert expired recommendation.
	_, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_preset_recommendations (id, job_id, current_preset, recommended_preset, oom_count, window_start, expires_at)
		 VALUES ($1, $2, 'micro', 'small-1x', 1, NOW() - INTERVAL '25 hours', NOW() - INTERVAL '1 hour')`,
		uuid.Must(uuid.NewV7()).String(), jobID1)
	if err != nil {
		t.Fatalf("insert expired: %v", err)
	}

	// Insert active recommendation.
	err = q.RecordOOMEvent(ctx, jobID2, "small-1x")
	if err != nil {
		t.Fatalf("RecordOOMEvent() error = %v", err)
	}

	deleted, err := q.CleanupExpiredRecommendations(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredRecommendations() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// Active recommendation should still exist.
	rec, err := q.GetPresetRecommendation(ctx, jobID2)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	if rec == nil {
		t.Error("active recommendation should still exist")
	}
}

func TestRecordOOMEvent_ConcurrentUpserts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	jobID := newID()
	var wg conc.WaitGroup
	var errs []error
	var mu sync.Mutex

	for range 5 {
		wg.Go(func() {
			if err := q.RecordOOMEvent(ctx, jobID, "micro"); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent upserts produced %d errors: %v", len(errs), errs[0])
	}

	rec, err := q.GetPresetRecommendation(ctx, jobID)
	if err != nil {
		t.Fatalf("GetPresetRecommendation() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected recommendation")
	}
	if rec.OOMCount != 5 {
		t.Errorf("oom_count = %d, want 5 from concurrent upserts", rec.OOMCount)
	}
}

// --- Compute Usage Integration Tests ---

func TestCreateRunComputeUsage_WithStatus(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := newID()
	now := time.Now()

	usage := &domain.RunComputeUsage{
		RunID:         newID(),
		ProjectID:     projectID,
		JobID:         newID(),
		MachinePreset: "micro",
		MachineID:     "m-123",
		DurationSecs:  10.5,
		CostMicrousd:  178,
		StartedAt:     &now,
		FinishedAt:    &now,
	}
	if err := q.CreateRunComputeUsage(ctx, usage); err != nil {
		t.Fatalf("CreateRunComputeUsage() error = %v", err)
	}

	got, err := q.GetRunComputeUsage(ctx, usage.ID)
	if err != nil {
		t.Fatalf("GetRunComputeUsage() error = %v", err)
	}
	if got.MachinePreset != "micro" {
		t.Errorf("preset = %q, want micro", got.MachinePreset)
	}
	if got.CostMicrousd != 178 {
		t.Errorf("cost = %d, want 178", got.CostMicrousd)
	}
}

// --- Signal Classification Tests (compute package, not store, but verifying chain) ---

func TestSignalClassification_OOMUpgradeChain(t *testing.T) {
	// Verify the full OOM upgrade chain: micro → small-1x → ... → large-2x.
	presets := compute.PresetOrder
	for i := 0; i < len(presets)-1; i++ {
		next, ok := compute.NextPreset(presets[i])
		if !ok {
			t.Errorf("NextPreset(%q) returned false", presets[i])
		}
		if next != presets[i+1] {
			t.Errorf("NextPreset(%q) = %q, want %q", presets[i], next, presets[i+1])
		}
	}

	// Verify max preset has no next.
	_, ok := compute.NextPreset("large-2x")
	if ok {
		t.Error("NextPreset(large-2x) should return false")
	}
}

func TestSignalClassification_ExitCodes(t *testing.T) {
	tests := []struct {
		code       int
		errorClass string
		isOOM      bool
	}{
		{137, "out_of_memory", true},
		{143, "graceful_shutdown", false},
		{139, "segfault", false},
		{1, "application_error", false},
		{2, "application_error", false},
		{127, "application_error", false},
		{255, "server", false},
		{-1, "server", false},
	}

	for _, tt := range tests {
		c := compute.ClassifyExitCode(tt.code)
		if c.ErrorClass != tt.errorClass {
			t.Errorf("code %d: ErrorClass = %q, want %q", tt.code, c.ErrorClass, tt.errorClass)
		}
		if c.IsOOM != tt.isOOM {
			t.Errorf("code %d: IsOOM = %v, want %v", tt.code, c.IsOOM, tt.isOOM)
		}
	}
}

// --- Region Failover Tests ---

func TestRegionFallbackChain_AllRegionsHaveChains(t *testing.T) {
	// Every known region should have a non-empty fallback chain.
	regions := []string{"iad", "ewr", "lhr", "nrt", "syd", "gru", "fra", "sin"}
	for _, r := range regions {
		chain := compute.RegionFallbackChain(r)
		if len(chain) == 0 {
			t.Errorf("region %q has no fallback chain", r)
		}
		// Verify no duplicates.
		seen := map[string]bool{r: true}
		for _, fb := range chain {
			if seen[fb] {
				t.Errorf("region %q: duplicate %q in chain", r, fb)
			}
			seen[fb] = true
		}
	}
}

// --- Helper ---

func mustCreateProjectQuota(t *testing.T, ctx context.Context, q *store.Queries, projectID string, limitMicrousd int64) {
	t.Helper()
	_, err := testDB.Pool.Exec(ctx,
		`INSERT INTO project_quotas (project_id, compute_daily_cost_limit_microusd)
		 VALUES ($1, $2)
		 ON CONFLICT (project_id) DO UPDATE SET compute_daily_cost_limit_microusd = $2`,
		projectID, limitMicrousd)
	if err != nil {
		t.Fatalf("create project quota: %v", err)
	}
}
