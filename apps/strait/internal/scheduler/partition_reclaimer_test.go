package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ PartitionReclaimerStore = (*fakeReclaimerStore)(nil)

type fakeReclaimerStore struct {
	jobPartitions    []string
	outboxPartitions []string
	rowCounts        map[string]int64
	estimatedCounts  map[string]int64
	dropped          []string
	ddlErr           error
	atomicDropCalled map[string]bool
}

func (f *fakeReclaimerStore) ListJobRunsPartitions(_ context.Context) ([]string, error) {
	return f.jobPartitions, nil
}

func (f *fakeReclaimerStore) ListOutboxHistoryPartitions(_ context.Context) ([]string, error) {
	return f.outboxPartitions, nil
}

func (f *fakeReclaimerStore) PartitionEstimatedRowCount(_ context.Context, name string) (int64, error) {
	if f.estimatedCounts != nil {
		if est, ok := f.estimatedCounts[name]; ok {
			return est, nil
		}
	}
	return 0, nil
}

func (f *fakeReclaimerStore) DropPartitionIfEmptyWithTimeout(_ context.Context, partition string, _ time.Duration) (bool, error) {
	if f.atomicDropCalled == nil {
		f.atomicDropCalled = make(map[string]bool)
	}
	f.atomicDropCalled[partition] = true
	if f.ddlErr != nil {
		return false, f.ddlErr
	}
	if f.rowCounts[partition] > 0 {
		return false, nil
	}
	f.dropped = append(f.dropped, partition)
	return true, nil
}

func TestPartitionReclaimer_SkipsRecentPartitions(t *testing.T) {
	now := time.Now().UTC()
	currentMonth := now.Format("2006_01")
	prevMonth := now.AddDate(0, -1, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions: []string{
			"job_runs_p" + currentMonth,
			"job_runs_p" + prevMonth,
		},
		rowCounts: map[string]int64{},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.Empty(t, s.dropped)
}

func TestPartitionReclaimer_DropsEmptyOldPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions: []string{"job_runs_p" + old},
		rowCounts:     map[string]int64{"job_runs_p" + old: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	require.Len(t, s.dropped,

		1)
}

func TestPartitionReclaimer_SkipsNonEmptyPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions: []string{"job_runs_p" + old},
		rowCounts:     map[string]int64{"job_runs_p" + old: 42},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.Empty(t, s.dropped)
}

func TestPartitionReclaimer_DropsOutboxHistoryPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions:    nil,
		outboxPartitions: []string{"enqueue_outbox_history_p" + old},
		rowCounts:        map[string]int64{"enqueue_outbox_history_p" + old: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	require.Len(t, s.dropped,

		1)
}

func TestPartitionReclaimer_InvalidPartitionName(t *testing.T) {
	s := &fakeReclaimerStore{
		jobPartitions: []string{
			"not_a_valid_partition",
			"job_runs_p_nope",
			"job_runs_pNOT_MONTH",
			"; DROP TABLE job_runs;--",
		},
		rowCounts: map[string]int64{},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 0})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.Empty(t, s.dropped)
}

func TestPartitionReclaimer_DDLError(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")
	ddlErr := fmt.Errorf("lock timeout")

	s := &fakeReclaimerStore{
		jobPartitions: []string{"job_runs_p" + old},
		rowCounts:     map[string]int64{"job_runs_p" + old: 0},
		ddlErr:        ddlErr,
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.EqualValues(t, 1,
		r.Errors())
	assert.EqualValues(t, 0,
		r.Dropped())
}

func TestPartitionReclaimer_IterationsIncrement(t *testing.T) {
	s := &fakeReclaimerStore{rowCounts: map[string]int64{}}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{})
	for range 3 {
		_ = r.RunOnceForTest(context.Background())
	}
	assert.EqualValues(t, 3,
		r.Iterations())
}

func TestPartitionReclaimer_BothTableTypes(t *testing.T) {
	oldJob := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")
	oldOutbox := time.Now().UTC().AddDate(0, -8, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions:    []string{"job_runs_p" + oldJob},
		outboxPartitions: []string{"enqueue_outbox_history_p" + oldOutbox},
		rowCounts: map[string]int64{
			"job_runs_p" + oldJob:                  0,
			"enqueue_outbox_history_p" + oldOutbox: 0,
		},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	require.Len(t, s.dropped,

		2)
}

func TestPartitionReclaimer_Defaults(t *testing.T) {
	r := NewPartitionReclaimer(&fakeReclaimerStore{rowCounts: map[string]int64{}}, PartitionReclaimerConfig{})
	assert.Equal(t, 24*
		time.
			Hour, r.interval,
	)
	assert.Equal(t, 2,
		r.safetyMonths,
	)
}

func TestPartitionReclaimer_SkipsOnEstimateNonEmpty(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")
	name := "job_runs_p" + old

	s := &fakeReclaimerStore{
		jobPartitions:   []string{name},
		rowCounts:       map[string]int64{name: 0},
		estimatedCounts: map[string]int64{name: 500},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.False(t, s.atomicDropCalled[name])
	assert.Empty(t, s.dropped)
}

func TestPartitionReclaimer_FallsThroughOnEstimateZero(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")
	name := "job_runs_p" + old

	s := &fakeReclaimerStore{
		jobPartitions:   []string{name},
		rowCounts:       map[string]int64{name: 0},
		estimatedCounts: map[string]int64{name: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	assert.True(t, s.atomicDropCalled[name])
	assert.Len(t, s.dropped,
		1,
	)
}

func TestPartitionReclaimer_UsesAtomicDropAfterEstimateZero(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")
	name := "job_runs_p" + old

	s := &fakeReclaimerStore{
		jobPartitions:   []string{name},
		rowCounts:       map[string]int64{name: 17},
		estimatedCounts: map[string]int64{name: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	require.NoError(t,
		r.RunOnceForTest(context.
			Background()))
	require.True(t, s.atomicDropCalled[name])
	require.Empty(t, s.dropped)
}
