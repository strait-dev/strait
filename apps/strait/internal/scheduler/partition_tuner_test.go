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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Fail(t,

			"missing current month")
	}
	if _, ok := hot["job_runs_p2026_03"]; !ok {
		assert.Fail(t,

			"missing previous month")
	}
	if _, ok := hot["job_runs_p2026_02"]; ok {
		assert.Fail(t,

			"should not include 2 months back")
	}
}

func TestPartitionTuner_HotPartitionNames_CrossYear(t *testing.T) {
	now := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hot := hotPartitionNames(now)
	if _, ok := hot["job_runs_p2026_01"]; !ok {
		assert.Fail(t,

			"missing 2026_01")
	}
	if _, ok := hot["job_runs_p2025_12"]; !ok {
		assert.Fail(t,

			"missing 2025_12 (prev across year boundary)")
	}
}

func TestPartitionTuner_HotPartitionNames_MonthEndIncludesPreviousMonth(t *testing.T) {
	now := time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)
	hot := hotPartitionNames(now)
	if _, ok := hot["job_runs_p2026_03"]; !ok {
		assert.Fail(t,

			"missing current month")
	}
	if _, ok := hot["job_runs_p2026_02"]; !ok {
		assert.Fail(t,

			"missing February as previous month on March 31")
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
	require.NoError(t,
		tu.runOnce(context.Background()))
	assert.Equal(t, 2,
		tu.HotCount())
	assert.Equal(t, 3,
		tu.ColdCount())

	// Verify the hot DDLs target the correct partitions. Filter on the
	// autovacuum_vacuum_scale_factor key to exclude fillfactor-only ALTERs.
	var hotDDLs int
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "autovacuum_vacuum_scale_factor") &&
			(strings.Contains(sql, "job_runs_p2026_04") || strings.Contains(sql, "job_runs_p2026_03")) {
			hotDDLs++
		}
	}
	assert.Equal(t, 2,
		hotDDLs)
}

func TestPartitionTuner_EmptyPartitionList(t *testing.T) {
	s := &fakeTunerStore{partitions: []string{}}
	tu := NewPartitionTuner(s, PartitionTunerConfig{})
	require.NoError(t,
		tu.runOnce(context.Background()))
	assert.False(t, tu.
		HotCount() !=
		0 || tu.
		ColdCount() != 0)
}

func TestPartitionTuner_ListError(t *testing.T) {
	s := &fakeTunerStore{listErr: errors.New("list down")}
	tu := NewPartitionTuner(s, PartitionTunerConfig{})
	assert.Error(t, tu.
		runOnce(context.
			Background()),
	)
}

func TestPartitionTuner_ExecErrorContinuesToNextPartition(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2020_01", "job_runs_p2026_04"},
		execErr:    errors.New("exec failed"),
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	_ = tu.runOnce(context.Background())
	assert.False(t, tu.
		HotCount() !=
		0 || tu.
		ColdCount() != 0)

	// Both partitions should have been attempted but failed; counters
	// should stay zero because neither succeeded.
}

func TestPartitionTuner_LockNotAcquired(t *testing.T) {
	s := &fakeTunerStore{partitions: []string{"job_runs_p2026_04"}}
	locker := &fakeLocker{acquireOK: false}
	tu := NewPartitionTuner(s, PartitionTunerConfig{}).WithAdvisoryLocker(locker)
	_ = tu.runOnce(context.Background())
	assert.Empty(t, s.DDLs())
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
		require.Fail(t, "did not exit on cancel")
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
	assert.Equal(t, 2,
		tu.HotCount())

	s.mu.Lock()
	s.ddls = nil
	s.mu.Unlock()

	// Now simulate month 5: hot = {04, 05}
	tu.clock = func() time.Time { return time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC) }
	_ = tu.runOnce(context.Background())
	assert.Equal(t, 2,
		tu.HotCount())

	// 03 should now be cold.
	var coldFor03 bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "job_runs_p2026_03") && strings.Contains(sql, "RESET") {
			coldFor03 = true
		}
	}
	assert.True(t, coldFor03)
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
	require.NoError(t,
		tu.runOnce(context.Background()))

	elapsed := time.Since(start)
	assert.LessOrEqual(
		t, elapsed,
		140*time.
			Millisecond,
	)
	assert.GreaterOrEqual(t, s.peak.
		Load(),
		int32(2))
	assert.LessOrEqual(
		t, s.peak.
			Load(), int32(partitionTunerPoolSize),
	)

	// Serial would be 16 * 10ms = 160ms. With pool size 4 we expect
	// ~40ms plus scheduling slack.

	s.mu.Lock()
	got := len(s.ddls)
	s.mu.Unlock()
	assert.Equal(t, 2*
		len(parts), got)
	assert.Equal(t, len(parts), tu.
		ColdCount())

	// Each partition emits one autovacuum DDL plus one fillfactor DDL.
}

func TestPartitionTuner_AppliesFillfactorWhenMissing(t *testing.T) {
	s := &fakeTunerStore{
		partitions: []string{"job_runs_p2026_04"},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	require.NoError(t,
		tu.runOnce(context.Background()))

	var fillffSet bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "fillfactor = 85") && strings.Contains(sql, "job_runs_p2026_04") {
			fillffSet = true
		}
	}
	require.True(t, fillffSet)
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
	require.NoError(t,
		tu.runOnce(context.Background()))

	for _, sql := range s.DDLs() {
		require.NotContains(t, sql, "fillfactor")
	}
}

func TestPartitionTuner_FillfactorAppliesToColdPartitions(t *testing.T) {
	s := &fakeTunerStore{
		// 2020_01 is far in the past, so it should be treated as cold.
		partitions: []string{"job_runs_p2020_01"},
	}
	clock := func() time.Time { return time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) }
	tu := NewPartitionTuner(s, PartitionTunerConfig{Clock: clock})
	require.NoError(t,
		tu.runOnce(context.Background()))
	require.Equal(t, 1,
		tu.ColdCount())

	var fillffSet bool
	for _, sql := range s.DDLs() {
		if strings.Contains(sql, "fillfactor = 85") {
			fillffSet = true
		}
	}
	require.True(t, fillffSet)
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
		assert.False(t, y !=
			c.wantY ||
			m != c.wantM,
		)
	}
}
