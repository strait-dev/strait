package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var _ PartitionReclaimerStore = (*fakeReclaimerStore)(nil)

type fakeReclaimerStore struct {
	jobPartitions    []string
	outboxPartitions []string
	rowCounts        map[string]int64
	dropped          []string
	ddlErr           error
}

func (f *fakeReclaimerStore) ListJobRunsPartitions(_ context.Context) ([]string, error) {
	return f.jobPartitions, nil
}

func (f *fakeReclaimerStore) ListOutboxHistoryPartitions(_ context.Context) ([]string, error) {
	return f.outboxPartitions, nil
}

func (f *fakeReclaimerStore) PartitionRowCount(_ context.Context, name string) (int64, error) {
	return f.rowCounts[name], nil
}

func (f *fakeReclaimerStore) DropPartitionWithTimeout(_ context.Context, partition string, _ time.Duration) error {
	if f.ddlErr != nil {
		return f.ddlErr
	}
	f.dropped = append(f.dropped, partition)
	return nil
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
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 0 {
		t.Errorf("expected no drops, got %v", s.dropped)
	}
}

func TestPartitionReclaimer_DropsEmptyOldPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions: []string{"job_runs_p" + old},
		rowCounts:     map[string]int64{"job_runs_p" + old: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 1 {
		t.Fatalf("expected 1 drop, got %d", len(s.dropped))
	}
}

func TestPartitionReclaimer_SkipsNonEmptyPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions: []string{"job_runs_p" + old},
		rowCounts:     map[string]int64{"job_runs_p" + old: 42},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 0 {
		t.Errorf("expected no drops for non-empty partition, got %v", s.dropped)
	}
}

func TestPartitionReclaimer_DropsOutboxHistoryPartition(t *testing.T) {
	old := time.Now().UTC().AddDate(0, -6, 0).Format("2006_01")

	s := &fakeReclaimerStore{
		jobPartitions:    nil,
		outboxPartitions: []string{"enqueue_outbox_history_p" + old},
		rowCounts:        map[string]int64{"enqueue_outbox_history_p" + old: 0},
	}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{SafetyMonths: 2})
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 1 {
		t.Fatalf("expected 1 drop for outbox history, got %d", len(s.dropped))
	}
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
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 0 {
		t.Errorf("expected no drops for invalid partition names, got %v", s.dropped)
	}
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
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Errors() != 1 {
		t.Errorf("errors = %d, want 1", r.Errors())
	}
	if r.Dropped() != 0 {
		t.Errorf("dropped = %d, want 0", r.Dropped())
	}
}

func TestPartitionReclaimer_IterationsIncrement(t *testing.T) {
	s := &fakeReclaimerStore{rowCounts: map[string]int64{}}
	r := NewPartitionReclaimer(s, PartitionReclaimerConfig{})
	for range 3 {
		_ = r.RunOnceForTest(context.Background())
	}
	if r.Iterations() != 3 {
		t.Errorf("iterations = %d, want 3", r.Iterations())
	}
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
	if err := r.RunOnceForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.dropped) != 2 {
		t.Fatalf("expected 2 drops (one per table type), got %d: %v", len(s.dropped), s.dropped)
	}
}

func TestPartitionReclaimer_Defaults(t *testing.T) {
	r := NewPartitionReclaimer(&fakeReclaimerStore{rowCounts: map[string]int64{}}, PartitionReclaimerConfig{})
	if r.interval != 24*time.Hour {
		t.Errorf("interval = %v", r.interval)
	}
	if r.safetyMonths != 2 {
		t.Errorf("safetyMonths = %d", r.safetyMonths)
	}
}
