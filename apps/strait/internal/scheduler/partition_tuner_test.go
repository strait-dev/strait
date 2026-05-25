package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

type fakeTunerStore struct {
	partitions []string
	mu         sync.Mutex
	ddls       []string
	listErr    error
	execErr    error
	missing    map[string]bool
	// reloptions maps partition -> option -> value. Used by
	// PartitionReloption to drive the tuner's idempotent fillfactor path.
	reloptions map[string]map[string]string
}

func (f *fakeTunerStore) ListJobRunsPartitions(_ context.Context) ([]string, error) {
	return f.partitions, f.listErr
}

func (f *fakeTunerStore) PartitionExists(_ context.Context, name string) (bool, error) {
	if f.missing != nil && f.missing[name] {
		return false, nil
	}
	return true, nil
}

func (f *fakeTunerStore) ExecDDL(_ context.Context, sql string) error {
	if f.execErr != nil {
		return f.execErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ddls = append(f.ddls, sql)
	return nil
}

func (f *fakeTunerStore) PartitionReloption(_ context.Context, name, option string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if opts, ok := f.reloptions[name]; ok {
		return opts[option], nil
	}
	return "", nil
}

func (f *fakeTunerStore) DDLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.ddls))
	copy(out, f.ddls)
	return out
}

func TestPartitionTuner_HotPartitionNames_CurrentAndPrev(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	hot := hotPartitionNames(now)
	if _, ok := hot["job_runs_p2026_04"]; !ok {
		t.Error("missing current month")
	}
	if _, ok := hot["job_runs_p2026_03"]; !ok {
		t.Error("missing previous month")
	}
	if _, ok := hot["job_runs_p2026_02"]; ok {
		t.Error("should not include 2 months back")
	}
}

func TestPartitionTuner_HotPartitionNames_CrossYear(t *testing.T) {
	now := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hot := hotPartitionNames(now)
	if _, ok := hot["job_runs_p2026_01"]; !ok {
		t.Error("missing 2026_01")
	}
	if _, ok := hot["job_runs_p2025_12"]; !ok {
		t.Error("missing 2025_12 (prev across year boundary)")
	}
}

func TestPartitionTuner_HotPartitionNames_MonthEndIncludesPreviousMonth(t *testing.T) {
	now := time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)
	hot := hotPartitionNames(now)
	if _, ok := hot["job_runs_p2026_03"]; !ok {
		t.Error("missing current month")
	}
	if _, ok := hot["job_runs_p2026_02"]; !ok {
		t.Error("missing February as previous month on March 31")
	}
}

func TestPartitionTuner_AppliesHotThenCold(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{
			"job_runs_p2026_01",
			"job_runs_p2026_02",
			"job_runs_p2026_03",
			"job_runs_p2026_04",
			"job_runs_p2026_05",
		},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if tu.HotCount() != 2 {
		t.Errorf("hot = %d, want 2", tu.HotCount())
	}
	if tu.ColdCount() != 3 {
		t.Errorf("cold = %d, want 3", tu.ColdCount())
	}
	// Verify the hot DDLs target the correct partitions. Filter on the
	// autovacuum_vacuum_scale_factor key to exclude fillfactor-only ALTERs.
	var hotDDLs int
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "autovacuum_vacuum_scale_factor") &&
			(strings.Contains(sql, "job_runs_p2026_04") || strings.Contains(sql, "job_runs_p2026_03")) {
			hotDDLs++
		}
	}
	if hotDDLs != 2 {
		t.Errorf("expected 2 hot SET DDLs, got %d (all ddls: %v)", hotDDLs, s.DDLs())
	}
}

func TestPartitionTuner_EmptyPartitionList(t *testing.T) {
	s := &fakeTunerStore{partitions: []string{}}
	tu := NewPartitionTuner(s, PartitionTunerConfig{})
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if tu.HotCount() != 0 || tu.ColdCount() != 0 {
		t.Errorf("empty should be no-op: hot=%d cold=%d", tu.HotCount(), tu.ColdCount())
	}
}

func TestPartitionTuner_ListError(t *testing.T) {
	s := &fakeTunerStore{listErr: errors.New("list down")}
	tu := NewPartitionTuner(s, PartitionTunerConfig{})
	if err := tu.runOnce(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestPartitionTuner_ExecErrorContinuesToNextPartition(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2020_01", "job_runs_p2026_04"},
		execErr:    errors.New("exec failed"),
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	_ = tu.runOnce(context.Background())
	// Both partitions should have been attempted but failed; counters
	// should stay zero because neither succeeded.
	if tu.HotCount() != 0 || tu.ColdCount() != 0 {
		t.Errorf("no successes expected: hot=%d cold=%d", tu.HotCount(), tu.ColdCount())
	}
}

func TestPartitionTuner_LockNotAcquired(t *testing.T) {
	s := &fakeTunerStore{partitions: []string{"job_runs_p2026_04"}}
	locker := &fakeLocker{acquireOK: false}
	tu := NewPartitionTuner(s, PartitionTunerConfig{}).WithAdvisoryLocker(locker)
	_ = tu.runOnce(context.Background())
	if len(s.DDLs()) != 0 {
		t.Errorf("should not exec without lock")
	}
}

func TestPartitionTuner_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakeTunerStore{}
	tu := NewPartitionTuner(s, PartitionTunerConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		tu.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not exit on cancel")
	}
}

func TestPartitionTuner_RotatesHotAsTimeAdvances(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2026_03", "job_runs_p2026_04", "job_runs_p2026_05"},
	}
	// Month 4: hot = {03, 04}, cold = {05}
	tu := NewPartitionTuner(s, PartitionTunerConfig{
		Clock: func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) },
	})
	_ = tu.runOnce(context.Background())
	if tu.HotCount() != 2 {
		t.Errorf("month 4 hot = %d, want 2", tu.HotCount())
	}
	s.mu.Lock()
	s.ddls = nil
	s.mu.Unlock()

	// Now simulate month 5: hot = {04, 05}
	tu.clock = func() time.Time { return time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC) }
	_ = tu.runOnce(context.Background())
	if tu.HotCount() != 2 {
		t.Errorf("month 5 hot = %d, want 2", tu.HotCount())
	}
	// 03 should now be cold.
	var coldFor03 bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "job_runs_p2026_03") && strings.Contains(sql, "RESET") {
			coldFor03 = true
		}
	}
	if !coldFor03 {
		t.Errorf("expected 2026_03 to be RESET after month rotation; ddls: %v", s.DDLs())
	}
}

// slowTunerStore exposes any ordering or race hazards in the
// parallelized ALTER path by sleeping inside ExecDDL and tracking the
// peak concurrent executors.
type slowTunerStore struct {
	mu         sync.Mutex
	partitions []string
	ddls       []string
	active     atomic.Int32
	peak       atomic.Int32
	delay      time.Duration
}

func (s *slowTunerStore) ListJobRunsPartitions(_ context.Context) ([]string, error) {
	return s.partitions, nil
}
func (s *slowTunerStore) PartitionExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (s *slowTunerStore) PartitionReloption(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (s *slowTunerStore) ExecDDL(_ context.Context, sql string) error {
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		peak := s.peak.Load()
		if n <= peak || s.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	time.Sleep(s.delay)
	s.mu.Lock()
	s.ddls = append(s.ddls, sql)
	s.mu.Unlock()
	return nil
}

func TestPartitionTuner_ParallelExec(t *testing.T) {
	parts := make([]string, 0, 16)
	for i := range 16 {
		parts = append(parts, "job_runs_p2020_"+string(rune('0'+i/10))+string(rune('0'+i%10)))
	}
	s := &slowTunerStore{partitions: parts, delay: 10 * time.Millisecond}
	tu := NewPartitionTuner(s, PartitionTunerConfig{
		Clock: func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) },
	})
	start := time.Now()
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	elapsed := time.Since(start)
	// Serial would be 16 * 10ms = 160ms. With pool size 4 we expect
	// ~40ms plus scheduling slack.
	if elapsed > 140*time.Millisecond {
		t.Errorf("expected parallel execution, elapsed=%v", elapsed)
	}
	if peak := s.peak.Load(); peak < 2 {
		t.Errorf("expected concurrent executors, peak=%d", peak)
	}
	if peak := s.peak.Load(); peak > partitionTunerPoolSize {
		t.Errorf("peak=%d exceeds pool size %d", peak, partitionTunerPoolSize)
	}
	s.mu.Lock()
	got := len(s.ddls)
	s.mu.Unlock()
	// Each partition emits one autovacuum DDL plus one fillfactor DDL.
	if got != 2*len(parts) {
		t.Errorf("ddls count = %d, want %d", got, 2*len(parts))
	}
	if tu.ColdCount() != len(parts) {
		t.Errorf("coldCount = %d, want %d", tu.ColdCount(), len(parts))
	}
}

func TestPartitionTuner_AppliesFillfactorWhenMissing(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2026_04"},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	var fillffSet bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "fillfactor = 85") && strings.Contains(sql, "job_runs_p2026_04") {
			fillffSet = true
		}
	}
	if !fillffSet {
		t.Fatalf("expected fillfactor DDL for partition; got: %v", s.DDLs())
	}
}

func TestPartitionTuner_SkipsFillfactorWhenAlreadySet(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2026_04"},
		reloptions: map[string]map[string]string{
			"job_runs_p2026_04": {"fillfactor": "85"},
		},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "fillfactor") {
			t.Fatalf("expected no fillfactor DDL when already set; got: %s", sql)
		}
	}
}

func TestPartitionTuner_FillfactorAppliesToColdPartitions(t *testing.T) {
	s := &fakeTunerStore{
		// 2020_01 is far in the past, so it should be treated as cold.
		partitions: []string{"job_runs_p2020_01"},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	if err := tu.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if tu.ColdCount() != 1 {
		t.Fatalf("ColdCount = %d, want 1", tu.ColdCount())
	}
	var fillffSet bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "fillfactor = 85") {
			fillffSet = true
		}
	}
	if !fillffSet {
		t.Fatalf("cold partition missing fillfactor DDL; got: %v", s.DDLs())
	}
}

func TestParsePartitionMonth(t *testing.T) {
	cases := []struct {
		name  string
		wantY int
		wantM int
	}{
		{"job_runs_p2026_04", 2026, 4},
		{"job_runs_p2020_12", 2020, 12},
		{"not_a_partition", 0, 0},
		{"job_runs_pXXXX_04", 0, 0},
	}
	for _, c := range cases {
		y, m := parsePartitionMonth(c.name)
		if y != c.wantY || m != c.wantM {
			t.Errorf("%s → (%d, %d), want (%d, %d)", c.name, y, m, c.wantY, c.wantM)
		}
	}
}
