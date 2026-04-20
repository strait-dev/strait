package scheduler

import (
	"context"
	"testing"
	"time"
)

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

func (f *fakeReclaimerStore) ExecDDL(_ context.Context, sql string) error {
	if f.ddlErr != nil {
		return f.ddlErr
	}
	f.dropped = append(f.dropped, sql)
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

func TestPartitionReclaimer_Defaults(t *testing.T) {
	r := NewPartitionReclaimer(&fakeReclaimerStore{rowCounts: map[string]int64{}}, PartitionReclaimerConfig{})
	if r.interval != 24*time.Hour {
		t.Errorf("interval = %v", r.interval)
	}
	if r.safetyMonths != 2 {
		t.Errorf("safetyMonths = %d", r.safetyMonths)
	}
}
