package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type createRunCaptureDB struct {
	query string
	args  []any
}

func (db *createRunCaptureDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected exec")
}

func (db *createRunCaptureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (db *createRunCaptureDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = append([]any(nil), args...)
	return createRunCreatedAtRow{createdAt: time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)}
}

type createRunCreatedAtRow struct {
	createdAt time.Time
}

func (r createRunCreatedAtRow) Scan(dest ...any) error {
	target, ok := dest[0].(*time.Time)
	if !ok {
		return errors.New("expected created_at scan target")
	}
	*target = r.createdAt
	return nil
}

func TestCreateRunPassesJobConfigSnapshot(t *testing.T) {
	t.Parallel()

	enabled := true
	paused := false
	maxConcurrency := 12
	maxConcurrencyPerKey := 3
	db := &createRunCaptureDB{}
	q := New(db)

	run := &domain.JobRun{
		ID:                      "run-1",
		JobID:                   "job-1",
		ProjectID:               "project-1",
		Status:                  domain.StatusQueued,
		Attempt:                 1,
		TriggeredBy:             domain.TriggerManual,
		JobEnabled:              &enabled,
		JobPaused:               &paused,
		JobMaxConcurrency:       &maxConcurrency,
		JobMaxConcurrencyPerKey: &maxConcurrencyPerKey,
	}
	require.NoError(t, q.CreateRun(context.Background(), run))

	require.Contains(t, db.query, "job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key")
	require.Len(t, db.args, 36)
	require.Same(t, &enabled, db.args[30])
	require.Same(t, &paused, db.args[31])
	require.Same(t, &maxConcurrency, db.args[32])
	require.Same(t, &maxConcurrencyPerKey, db.args[33])
	require.Equal(t, []byte("{}"), db.args[34])
	require.Equal(t, false, db.args[35])
}
