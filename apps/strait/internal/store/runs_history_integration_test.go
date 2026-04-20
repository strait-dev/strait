//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestIntegration_ArchiveTerminalRunRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := New(testDB.Pool)

	projectID := "proj-archive-rt-" + t.Name()
	job := baseJob("job-archive-rt", projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	run := baseRun(job, "run-archive-rt")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := q.ArchiveTerminalRun(ctx, tx, run.ID); err != nil {
		t.Fatalf("ArchiveTerminalRun: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	gotHot, err := q.GetRun(ctx, run.ID)
	if gotHot != nil {
		t.Error("run should be gone from hot table after archive")
	}
	if err != nil && err != ErrRunNotFound {
		t.Fatalf("GetRun (hot): unexpected error: %v", err)
	}

	gotHistory, err := q.GetRunFromHistory(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunFromHistory: %v", err)
	}
	if gotHistory == nil {
		t.Fatal("run should exist in history after archive")
	}
	if gotHistory.ID != run.ID {
		t.Errorf("history run ID = %q, want %q", gotHistory.ID, run.ID)
	}
	if gotHistory.Status != domain.StatusCompleted {
		t.Errorf("history run status = %q, want %q", gotHistory.Status, domain.StatusCompleted)
	}
}

func TestIntegration_ArchiveIdempotent(t *testing.T) {
	ctx := context.Background()
	q := New(testDB.Pool)

	projectID := "proj-archive-idem-" + t.Name()
	job := baseJob("job-archive-idem", projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	run := baseRun(job, "run-archive-idem")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	tx1, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := q.ArchiveTerminalRun(ctx, tx1, run.ID); err != nil {
		t.Fatalf("first ArchiveTerminalRun: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck
	if err := q.ArchiveTerminalRun(ctx, tx2, run.ID); err != nil {
		t.Fatalf("second ArchiveTerminalRun should not fail: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestIntegration_ArchiveTerminalRunsBatchAndRetention(t *testing.T) {
	ctx := context.Background()
	q := New(testDB.Pool)

	projectID := "proj-archive-batch-" + t.Name()
	job := baseJob("job-archive-batch", projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	past := time.Now().UTC().Add(-48 * time.Hour)
	for i := range 5 {
		run := baseRun(job, "run-archive-batch-"+string(rune('a'+i)))
		run.Status = domain.StatusCompleted
		finished := past.Add(time.Duration(i) * time.Minute)
		run.FinishedAt = &finished
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun %d: %v", i, err)
		}
	}

	archived, err := q.ArchiveTerminalRunsPastRetention(ctx, time.Hour, time.Hour, 10)
	if err != nil {
		t.Fatalf("ArchiveTerminalRunsPastRetention: %v", err)
	}
	if archived != 5 {
		t.Errorf("archived = %d, want 5", archived)
	}

	deleted, err := q.DeleteHistoryRunsPastRetention(ctx, time.Now().Add(time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteHistoryRunsPastRetention: %v", err)
	}
	if deleted < 5 {
		t.Errorf("deleted = %d, want >= 5", deleted)
	}
}

func TestIntegration_HistoryTableColumnSync(t *testing.T) {
	ctx := context.Background()

	type colInfo struct {
		Name string
		Type string
	}

	fetchColumns := func(table string) (map[string]colInfo, error) {
		rows, err := testDB.Pool.Query(ctx, `
			SELECT column_name, data_type
			FROM information_schema.columns
			WHERE table_name = $1
			ORDER BY ordinal_position`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		cols := make(map[string]colInfo)
		for rows.Next() {
			var c colInfo
			if err := rows.Scan(&c.Name, &c.Type); err != nil {
				return nil, err
			}
			cols[c.Name] = c
		}
		return cols, rows.Err()
	}

	hotCols, err := fetchColumns("job_runs")
	if err != nil {
		t.Fatalf("fetch job_runs columns: %v", err)
	}
	historyCols, err := fetchColumns("job_runs_history")
	if err != nil {
		t.Fatalf("fetch job_runs_history columns: %v", err)
	}

	for name, hot := range hotCols {
		hist, ok := historyCols[name]
		if !ok {
			t.Errorf("column %q exists in job_runs but not in job_runs_history", name)
			continue
		}
		if hot.Type != hist.Type {
			t.Errorf("column %q type mismatch: job_runs=%q, history=%q", name, hot.Type, hist.Type)
		}
	}

	allowed := map[string]bool{"archived_at": true}
	for name := range historyCols {
		if _, ok := hotCols[name]; !ok && !allowed[name] {
			t.Errorf("column %q exists in job_runs_history but not in job_runs (and not in allowed set)", name)
		}
	}
}

func TestIntegration_BackfillTerminalRunsToHistory(t *testing.T) {
	ctx := context.Background()
	q := New(testDB.Pool)

	projectID := "proj-backfill-" + t.Name()
	job := baseJob("job-backfill", projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	past := time.Now().UTC().Add(-72 * time.Hour)
	for i := range 3 {
		run := baseRun(job, "run-backfill-"+string(rune('a'+i)))
		run.Status = domain.StatusFailed
		finished := past.Add(time.Duration(i) * time.Hour)
		run.FinishedAt = &finished
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun %d: %v", i, err)
		}
	}

	activeRun := baseRun(job, "run-backfill-active")
	activeRun.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, activeRun); err != nil {
		t.Fatalf("CreateRun active: %v", err)
	}

	moved, err := q.BackfillTerminalRunsToHistory(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("BackfillTerminalRunsToHistory: %v", err)
	}
	if moved != 3 {
		t.Errorf("moved = %d, want 3", moved)
	}

	active, err := q.GetRun(ctx, activeRun.ID)
	if err != nil {
		t.Fatalf("GetRun active: %v", err)
	}
	if active.Status != domain.StatusQueued {
		t.Errorf("active run status = %q, want queued", active.Status)
	}
}

func TestIntegration_RepairOrphanedHistoryRuns(t *testing.T) {
	ctx := context.Background()
	q := New(testDB.Pool)

	projectID := "proj-repair-" + t.Name()
	job := baseJob("job-repair", projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	run := baseRun(job, "run-repair-dupe")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs_history (`+historyArchiveColumns+`)
		SELECT `+historyArchiveColumns+` FROM job_runs WHERE id = $1`, run.ID)
	if err != nil {
		t.Fatalf("manual insert into history: %v", err)
	}

	repaired, err := q.RepairOrphanedHistoryRuns(ctx, 10)
	if err != nil {
		t.Fatalf("RepairOrphanedHistoryRuns: %v", err)
	}
	if repaired != 1 {
		t.Errorf("repaired = %d, want 1", repaired)
	}

	dupes, err := q.CountDuplicateHistoryRuns(ctx)
	if err != nil {
		t.Fatalf("CountDuplicateHistoryRuns: %v", err)
	}
	if dupes != 0 {
		t.Errorf("dupes after repair = %d, want 0", dupes)
	}
}
