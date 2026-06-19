package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func workerScanFn(now time.Time) func(dest ...any) error {
	return func(dest ...any) error {
		*(dest[0].(*string)) = "worker-1"
		*(dest[1].(*string)) = "project-1"
		*(dest[2].(*string)) = "critical"
		*(dest[3].(*string)) = "host-1"
		*(dest[4].(*string)) = "v1"
		*(dest[5].(*string)) = string(domain.WorkerStatusActive)
		*(dest[6].(*time.Time)) = now
		*(dest[7].(*time.Time)) = now.Add(-time.Minute)
		return nil
	}
}

func workerTaskScanFn(now time.Time, withResult bool) func(dest ...any) error {
	return func(dest ...any) error {
		*(dest[0].(*string)) = "task-1"
		*(dest[1].(*string)) = "worker-1"
		*(dest[2].(*string)) = "run-1"
		*(dest[3].(*string)) = "project-1"
		*(dest[4].(*int)) = 2
		*(dest[5].(*string)) = string(domain.WorkerTaskStatusResultReceived)
		*(dest[6].(*time.Time)) = now.Add(-time.Minute)
		acceptedAt := now.Add(-30 * time.Second)
		finishedAt := now
		*(dest[7].(**time.Time)) = &acceptedAt
		*(dest[8].(**time.Time)) = &finishedAt
		if !withResult {
			return nil
		}
		status := "success"
		errText := "none"
		duration := int64(42)
		receivedAt := now.Add(-10 * time.Second)
		*(dest[9].(**string)) = &status
		*(dest[10].(*[]byte)) = []byte(`{"ok":true}`)
		*(dest[11].(**string)) = &errText
		*(dest[12].(**int64)) = &duration
		*(dest[13].(**time.Time)) = &receivedAt
		return nil
	}
}

func TestWorkerRefsAndRegistrationUnit(t *testing.T) {
	t.Parallel()

	t.Run("normalizes active worker refs", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, activeWorkerRefsFromIDs(nil))
		require.Equal(t, []ActiveWorkerRef{{WorkerID: "worker-1"}, {WorkerID: "worker-2"}}, activeWorkerRefsFromIDs([]string{"worker-1", "", "worker-2"}))

		ids, projects := activeWorkerRefArrays([]ActiveWorkerRef{
			{WorkerID: "worker-1", ProjectID: "project-1"},
			{WorkerID: "", ProjectID: "project-ignored"},
			{WorkerID: "worker-ignored", ProjectID: ""},
			{WorkerID: "worker-2", ProjectID: "project-2"},
		})
		require.Equal(t, []string{"worker-1", "worker-2"}, ids)
		require.Equal(t, []string{"project-1", "project-2"}, projects)
	})

	t.Run("registers and updates worker status", func(t *testing.T) {
		t.Parallel()

		execCalls := 0
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execCalls++
				if execCalls == 1 {
					require.Contains(t, sql, "INSERT INTO workers")
					require.Contains(t, sql, "ON CONFLICT (project_id, id)")
					require.Equal(t, []any{"worker-1", "project-1", "critical", "host-1", "v1", "active"}, args)
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				}
				require.Contains(t, sql, "UPDATE workers SET status")
				require.Equal(t, []any{"draining", "worker-1", "project-1"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		q := New(db)

		require.NoError(t, q.RegisterWorker(context.Background(), &domain.Worker{
			ID:        "worker-1",
			ProjectID: "project-1",
			QueueName: "critical",
			Hostname:  "host-1",
			Version:   "v1",
			Status:    domain.WorkerStatusActive,
		}))
		require.NoError(t, q.SetWorkerStatus(context.Background(), "worker-1", "project-1", domain.WorkerStatusDraining))
	})

	t.Run("gets worker project and worker row", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "SELECT project_id FROM workers"):
					require.Equal(t, []any{"worker-1"}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "project-1"
						return nil
					}}
				case strings.Contains(sql, "FROM workers WHERE id = $1 AND project_id = $2"):
					require.Equal(t, []any{"worker-1", "project-1"}, args)
					return &mockRow{scanFn: workerScanFn(now)}
				default:
					require.Failf(t, "unexpected query", "sql=%s args=%v", sql, args)
					return &mockRow{}
				}
			},
		}
		q := New(db)

		projectID, ok, err := q.GetWorkerProjectByID(context.Background(), "worker-1")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "project-1", projectID)

		worker, err := q.GetWorker(context.Background(), "worker-1", "project-1")
		require.NoError(t, err)
		require.Equal(t, domain.WorkerStatusActive, worker.Status)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		projectID, ok, err = q.GetWorkerProjectByID(context.Background(), "missing")
		require.NoError(t, err)
		require.False(t, ok)
		require.Empty(t, projectID)
	})

	t.Run("lists workers with optional queue filter", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "queue_name = $2")
				require.Contains(t, sql, "LIMIT $3 OFFSET $4")
				require.Equal(t, []any{"project-1", "critical", 10, 5}, args)
				return &mockRows{scanFns: []func(dest ...any) error{workerScanFn(now)}}, nil
			},
		}
		got, err := New(db).ListWorkers(context.Background(), "project-1", "critical", 10, 5)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "worker-1", got[0].ID)
	})
}

func TestWorkerEvictionAndRecoverySelectorsUnit(t *testing.T) {
	t.Parallel()

	t.Run("evicts stale workers except exact active refs", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "unnest($2::text[], $3::text[])")
				require.Equal(t, cutoff, args[0])
				require.Equal(t, []string{"worker-1"}, args[1])
				require.Equal(t, []string{"project-1"}, args[2])
				return pgconn.NewCommandTag("UPDATE 3"), nil
			},
		}
		got, err := New(db).EvictStaleWorkersExceptRefs(context.Background(), cutoff, []ActiveWorkerRef{{WorkerID: "worker-1", ProjectID: "project-1"}})
		require.NoError(t, err)
		require.Equal(t, int64(3), got)
	})

	t.Run("requires transactions for stale recovery and disconnected requeue", func(t *testing.T) {
		t.Parallel()

		_, err := New(&mockDBTX{}).RecoverStaleWorkerTasks(context.Background(), time.Now(), "stale")
		require.ErrorContains(t, err, "requires transaction support")
		_, err = New(&mockDBTX{}).RequeueOpenWorkerTasks(context.Background(), "worker-1", "project-1", "disconnected")
		require.ErrorContains(t, err, "requires transaction support")
	})

	t.Run("lists recoverable stale worker task run ids", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "stale_workers")
				require.Contains(t, sql, "open_tasks")
				require.Equal(t, cutoff, args[0])
				require.Equal(t, []string{"worker-1"}, args[1])
				require.Equal(t, []string{"project-1"}, args[2])
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "run-1"
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListRecoverableStaleWorkerTaskRunIDs(context.Background(), cutoff, []ActiveWorkerRef{{WorkerID: "worker-1", ProjectID: "project-1"}})
		require.NoError(t, err)
		require.Equal(t, []string{"run-1"}, got)
	})

	t.Run("renews stream leases and deletes stale offline workers", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		expiresAt := cutoff.Add(time.Minute)
		execCalls := 0
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execCalls++
				if execCalls == 1 {
					require.Contains(t, sql, "stream_lease_expires_at")
					require.Equal(t, []any{expiresAt, "worker-1", "project-1"}, args)
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
				require.Contains(t, sql, "DELETE FROM workers")
				require.Equal(t, []any{cutoff}, args)
				return pgconn.NewCommandTag("DELETE 2"), nil
			},
		}
		q := New(db)
		require.NoError(t, q.RenewWorkerStreamLease(context.Background(), "worker-1", "project-1", expiresAt))
		deleted, err := q.DeleteStaleOfflineWorkers(context.Background(), cutoff)
		require.NoError(t, err)
		require.Equal(t, int64(2), deleted)
	})
}

func TestWorkerTaskLifecycleUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates task with default attempt and updates statuses", func(t *testing.T) {
		t.Parallel()

		execCalls := 0
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execCalls++
				switch execCalls {
				case 1:
					require.Contains(t, sql, "INSERT INTO worker_tasks")
					require.Equal(t, []any{"task-1", "worker-1", "run-1", "project-1", 1, "assigned"}, args)
				case 2:
					require.Contains(t, sql, "accepted_at = NOW()")
					require.Equal(t, []any{"accepted", "task-1"}, args)
				case 3:
					require.Contains(t, sql, "finished_at = NOW()")
					require.Equal(t, []any{"completed", "task-1"}, args)
				default:
					require.Failf(t, "unexpected exec", "call=%d sql=%s args=%v", execCalls, sql, args)
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		q := New(db)
		task := &domain.WorkerTask{ID: "task-1", WorkerID: "worker-1", RunID: "run-1", ProjectID: "project-1", Status: domain.WorkerTaskStatusAssigned}
		require.NoError(t, q.CreateWorkerTask(context.Background(), task))
		require.Equal(t, 1, task.Attempt)
		require.NoError(t, q.UpdateWorkerTaskStatus(context.Background(), "task-1", domain.WorkerTaskStatusAccepted))
		require.NoError(t, q.UpdateWorkerTaskStatus(context.Background(), "task-1", domain.WorkerTaskStatusCompleted))
	})

	t.Run("marks result received idempotently", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "SELECT EXISTS")
				require.Equal(t, []any{"result_received", "task-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			},
		}
		marked, err := New(db).MarkWorkerTaskResultReceived(context.Background(), "task-1")
		require.NoError(t, err)
		require.True(t, marked)
	})

	t.Run("records exact assignment result and rejects invalid attempts", func(t *testing.T) {
		t.Parallel()

		var captured []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "result_received_at = NOW()")
				captured = append([]any(nil), args...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		q := New(db)
		marked, err := q.MarkWorkerTaskResultReceivedByAssignment(context.Background(), "task-1", "worker-1", "project-1", "run-1", 0, "success", "", []byte(`{"ignored":true}`), 7)
		require.NoError(t, err)
		require.False(t, marked)

		marked, err = q.MarkWorkerTaskResultReceivedByAssignment(context.Background(), "task-1", "worker-1", "project-1", "run-1", 2, "success", "", []byte(`{"ok":true}`), 7)
		require.NoError(t, err)
		require.True(t, marked)
		require.Equal(t, "result_received", captured[0])
		require.Equal(t, "success", captured[1])
		require.Empty(t, captured[2])
		require.JSONEq(t, `{"ok":true}`, string(captured[3].(json.RawMessage)))
		require.Equal(t, int64(7), captured[4])
		require.Equal(t, 2, captured[9])
	})

	t.Run("marks latest open task by run id", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "ORDER BY assigned_at DESC")
				require.Equal(t, []any{"worker-1", "project-1", "run-1", "result_received"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		marked, err := New(db).MarkOpenWorkerTaskResultReceivedByRunID(context.Background(), "worker-1", "project-1", "run-1")
		require.NoError(t, err)
		require.True(t, marked)
	})
}

func TestWorkerTaskLookupAndRecoveryUnit(t *testing.T) {
	t.Parallel()

	t.Run("claims recoverable task results with default limit", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cutoff := now.Add(-time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FOR UPDATE OF wt SKIP LOCKED")
				require.Equal(t, []any{"result_received", cutoff, 100, "finalizing"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{workerTaskScanFn(now, true)}}, nil
			},
		}
		got, err := New(db).ClaimRecoverableWorkerTaskResults(context.Background(), cutoff, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, domain.WorkerTaskStatusResultReceived, got[0].Status)
		require.NotNil(t, got[0].Result)
		require.JSONEq(t, `{"ok":true}`, string(got[0].Result.Output))
	})

	t.Run("gets task and open assignments", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		calls := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				calls++
				switch calls {
				case 1:
					require.Contains(t, sql, "FROM worker_tasks WHERE id = $1")
					require.Equal(t, []any{"task-1"}, args)
					return &mockRow{scanFn: workerTaskScanFn(now, true)}
				case 2:
					require.Contains(t, sql, "AND attempt = $5")
					require.Equal(t, []any{"task-1", "worker-1", "project-1", "run-1", 2}, args)
					return &mockRow{scanFn: workerTaskScanFn(now, false)}
				case 3:
					require.Contains(t, sql, "ORDER BY assigned_at DESC")
					require.Equal(t, []any{"worker-1", "project-1", "run-1"}, args)
					return &mockRow{scanFn: workerTaskScanFn(now, false)}
				default:
					require.Failf(t, "unexpected query", "call=%d sql=%s args=%v", calls, sql, args)
					return &mockRow{}
				}
			},
		}
		q := New(db)
		task, err := q.GetWorkerTask(context.Background(), "task-1")
		require.NoError(t, err)
		require.Equal(t, "task-1", task.ID)
		require.NotNil(t, task.Result)

		task, err = q.GetOpenWorkerTaskByAssignment(context.Background(), "task-1", "worker-1", "project-1", "run-1", 2)
		require.NoError(t, err)
		require.Equal(t, "task-1", task.ID)

		task, err = q.GetOpenWorkerTaskByRunID(context.Background(), "worker-1", "project-1", "run-1")
		require.NoError(t, err)
		require.Equal(t, "task-1", task.ID)

		task, err = q.GetOpenWorkerTaskByAssignment(context.Background(), "task-1", "worker-1", "project-1", "run-1", 0)
		require.NoError(t, err)
		require.Nil(t, task)
	})

	t.Run("lists worker tasks by worker with status filter", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "project_id = $2")
				require.Contains(t, sql, "status = $3")
				require.Equal(t, []any{"worker-1", "project-1", "assigned", 10, 5}, args)
				return &mockRows{scanFns: []func(dest ...any) error{workerTaskScanFn(now, false)}}, nil
			},
		}
		got, err := New(db).ListWorkerTasksByWorker(context.Background(), "worker-1", "project-1", domain.WorkerTaskStatusAssigned, 10, 5)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "project-1", got[0].ProjectID)
	})

	t.Run("resets finalizing task for retry", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "status = $3")
				require.Equal(t, []any{"result_received", "task-1", "finalizing"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		require.NoError(t, New(db).ResetWorkerTaskFinalizingToResultReceived(context.Background(), "task-1"))
	})
}
