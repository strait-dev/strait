package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func eventTriggerScanFn(now time.Time, withOptionalFields bool) func(dest ...any) error {
	return func(dest ...any) error {
		*(dest[0].(*string)) = "trigger-1"
		*(dest[1].(*string)) = "event.key"
		*(dest[2].(*string)) = "project-1"
		*(dest[4].(*string)) = "job_run"
		*(dest[8].(*string)) = domain.EventTriggerStatusWaiting
		*(dest[11].(*int)) = 30
		*(dest[12].(*time.Time)) = now
		*(dest[14].(*time.Time)) = now.Add(time.Minute)

		if !withOptionalFields {
			return nil
		}

		environmentID := "env-prod"
		workflowRunID := "workflow-run-1"
		workflowStepRunID := "step-run-1"
		jobRunID := "job-run-1"
		errText := "failed"
		notifyURL := "https://example.com/hook"
		notifyStatus := "pending"
		triggerType := "event"
		sentBy := "api-key-1"
		receivedAt := now.Add(30 * time.Second)

		*(dest[3].(**string)) = &environmentID
		*(dest[5].(**string)) = &workflowRunID
		*(dest[6].(**string)) = &workflowStepRunID
		*(dest[7].(**string)) = &jobRunID
		*(dest[9].(*[]byte)) = []byte(`{"request":true}`)
		*(dest[10].(*[]byte)) = []byte(`{"response":true}`)
		*(dest[13].(**time.Time)) = &receivedAt
		*(dest[15].(**string)) = &errText
		*(dest[16].(**string)) = &notifyURL
		*(dest[17].(**string)) = &notifyStatus
		*(dest[18].(**string)) = &triggerType
		*(dest[19].(**string)) = &sentBy

		return nil
	}
}

type eventTriggerTx struct {
	*fakeTx
	commits   int
	rollbacks int
}

func (tx *eventTriggerTx) Commit(context.Context) error {
	tx.commits++
	return nil
}

func (tx *eventTriggerTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type eventTriggerBeginner struct {
	mockDBTX
	tx *eventTriggerTx
}

func (b *eventTriggerBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
}

func newReceiveEventTx(t *testing.T, payload json.RawMessage, failStep string, failErr error) *eventTriggerTx {
	t.Helper()

	var execCalls int
	tx := &eventTriggerTx{}
	tx.fakeTx = &fakeTx{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls++
			switch {
			case strings.Contains(sql, "UPDATE event_triggers"):
				require.Len(t, args, 6)
				require.Equal(t, domain.EventTriggerStatusReceived, args[0])
				if len(payload) == 0 {
					require.Nil(t, args[1])
				} else {
					require.Equal(t, payload, args[1])
				}
				require.NotNil(t, args[2])
				require.Nil(t, args[3])
				require.Equal(t, "trigger-1", args[4])
				require.Equal(t, domain.EventTriggerStatusWaiting, args[5])
				if failStep == "trigger" {
					return pgconn.CommandTag{}, failErr
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			case strings.Contains(sql, "UPDATE job_run_state"):
				require.Equal(t, []any{domain.StatusQueued, "run-1", domain.StatusWaiting}, args)
				if failStep == "run-state-error" {
					return pgconn.CommandTag{}, failErr
				}
				if failStep == "run-state-conflict" {
					return pgconn.NewCommandTag("UPDATE 0"), nil
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			case strings.Contains(sql, "INSERT INTO job_run_cache_versions"):
				require.Equal(t, []any{"run-1"}, args)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			case strings.Contains(sql, "INSERT INTO job_run_lifecycle_events"):
				require.Equal(t, "run-1", args[0])
				require.Equal(t, domain.StatusWaiting, args[1])
				require.Equal(t, domain.StatusQueued, args[2])
				require.Equal(t, 2, args[3])
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			default:
				require.Failf(t, "unexpected exec", "sql=%s args=%v call=%d", sql, args, execCalls)
				return pgconn.CommandTag{}, nil
			}
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "SELECT attempt")
			require.Equal(t, []any{"run-1"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*int)) = 2
				return nil
			}}
		},
	}
	return tx
}

func receiveEventCheckpointTx(t *testing.T, failErr error) *eventTriggerTx {
	t.Helper()

	var queryRows int
	tx := &eventTriggerTx{}
	tx.fakeTx = &fakeTx{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			queryRows++
			switch {
			case strings.Contains(sql, "FOR UPDATE"):
				require.Equal(t, []any{"run-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "run-1"
					return nil
				}}
			case strings.Contains(sql, "INSERT INTO run_checkpoints"):
				require.Equal(t, "run-1", args[0])
				require.Equal(t, "event_trigger", args[2])
				require.JSONEq(t, `{"checkpoint":true}`, string(args[3].(json.RawMessage)))
				return &mockRow{scanFn: func(dest ...any) error {
					if failErr != nil {
						return failErr
					}
					*(dest[0].(*int)) = 1
					*(dest[1].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			default:
				require.Failf(t, "unexpected query row", "sql=%s args=%v call=%d", sql, args, queryRows)
				return &mockRow{}
			}
		},
	}
	return tx
}

func TestEventTriggerCreateAndLookupUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates with defaults and nullable fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO event_triggers")
				args = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		trigger := &domain.EventTrigger{
			ID:          "trigger-1",
			EventKey:    "event.key",
			ProjectID:   "project-1",
			SourceType:  "job_run",
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 30,
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Minute),
		}

		require.NoError(t, New(db).CreateEventTrigger(context.Background(), trigger))
		require.Len(t, args, 20)
		require.Nil(t, args[3])
		require.Nil(t, args[5])
		require.Nil(t, args[6])
		require.Nil(t, args[7])
		require.Nil(t, args[9])
		require.Nil(t, args[10])
		require.Nil(t, args[15])
		require.Nil(t, args[16])
		require.Empty(t, args[17])
		require.Equal(t, "event", args[18])
		require.Empty(t, args[19])
	})

	t.Run("maps duplicate key to conflict", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
			},
		}

		err := New(db).CreateEventTrigger(context.Background(), &domain.EventTrigger{})
		require.ErrorIs(t, err, ErrEventKeyConflict)
	})

	t.Run("wraps create errors", func(t *testing.T) {
		t.Parallel()

		createErr := errors.New("insert failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, createErr
			},
		}

		err := New(db).CreateEventTrigger(context.Background(), &domain.EventTrigger{})
		require.ErrorContains(t, err, "create event trigger")
		require.ErrorIs(t, err, createErr)
	})

	t.Run("lookup variants handle rows missing and scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			call       func(*Queries) (*domain.EventTrigger, error)
			wantSQL    string
			wantArgs   []any
			wantWrap   string
			noRowsNil  bool
			withResult bool
		}{
			{
				name: "event key success",
				call: func(q *Queries) (*domain.EventTrigger, error) {
					return q.GetEventTriggerByEventKey(context.Background(), "event.key")
				},
				wantSQL:    "WHERE event_key = $1",
				wantArgs:   []any{"event.key"},
				wantWrap:   "get event trigger by event key",
				withResult: true,
			},
			{
				name: "event key for project missing",
				call: func(q *Queries) (*domain.EventTrigger, error) {
					return q.GetEventTriggerByEventKeyForProject(context.Background(), "event.key", "project-1")
				},
				wantSQL:   "WHERE event_key = $1 AND project_id = $2",
				wantArgs:  []any{"event.key", "project-1"},
				wantWrap:  "get event trigger by event key for project",
				noRowsNil: true,
			},
			{
				name: "step run scan error",
				call: func(q *Queries) (*domain.EventTrigger, error) {
					return q.GetEventTriggerByStepRunID(context.Background(), "step-run-1")
				},
				wantSQL:  "WHERE workflow_step_run_id = $1",
				wantArgs: []any{"step-run-1"},
				wantWrap: "get event trigger by step run id",
			},
			{
				name: "job run scan error",
				call: func(q *Queries) (*domain.EventTrigger, error) {
					return q.GetEventTriggerByJobRunID(context.Background(), "job-run-1")
				},
				wantSQL:  "WHERE job_run_id = $1",
				wantArgs: []any{"job-run-1"},
				wantWrap: "get event trigger by job run id",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				now := time.Now().UTC()
				scanErr := errors.New("scan failed")
				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
						require.Contains(t, sql, tc.wantSQL)
						require.Equal(t, tc.wantArgs, args)
						if tc.withResult {
							return &mockRow{scanFn: eventTriggerScanFn(now, true)}
						}
						if tc.noRowsNil {
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						}
						return &mockRow{scanFn: func(...any) error { return scanErr }}
					},
				}

				got, err := tc.call(New(db))
				if tc.withResult {
					require.NoError(t, err)
					require.Equal(t, "trigger-1", got.ID)
					require.Equal(t, "env-prod", got.EnvironmentID)
					require.Equal(t, "workflow-run-1", got.WorkflowRunID)
					require.Equal(t, "step-run-1", got.WorkflowStepRunID)
					require.Equal(t, "job-run-1", got.JobRunID)
					require.JSONEq(t, `{"request":true}`, string(got.RequestPayload))
					require.JSONEq(t, `{"response":true}`, string(got.ResponsePayload))
					require.Equal(t, "failed", got.Error)
					require.Equal(t, "https://example.com/hook", got.NotifyURL)
					require.Equal(t, "pending", got.NotifyStatus)
					require.Equal(t, "event", got.TriggerType)
					require.Equal(t, "api-key-1", got.SentBy)
					return
				}
				if tc.noRowsNil {
					require.NoError(t, err)
					require.Nil(t, got)
					return
				}
				require.ErrorContains(t, err, tc.wantWrap)
				require.ErrorIs(t, err, scanErr)
			})
		}
	})
}

func TestEventTriggerUpdateUnit(t *testing.T) {
	t.Parallel()

	t.Run("updates status and maps not found", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "UPDATE event_triggers")
				args = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		require.NoError(t, New(db).UpdateEventTriggerStatus(context.Background(), "trigger-1", domain.EventTriggerStatusReceived, json.RawMessage(`{"ok":true}`), &now, ""))
		require.Equal(t, []any{domain.EventTriggerStatusReceived, json.RawMessage(`{"ok":true}`), &now, nil, "trigger-1"}, args)

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		err := New(db).UpdateEventTriggerStatus(context.Background(), "missing", domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorContains(t, err, "event trigger not found")

		updateErr := errors.New("update failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, updateErr
		}
		err = New(db).UpdateEventTriggerStatus(context.Background(), "trigger-1", domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorContains(t, err, "update event trigger status")
		require.ErrorIs(t, err, updateErr)
	})

	t.Run("updates status from and distinguishes conflicts", func(t *testing.T) {
		t.Parallel()

		var queryRowErr error
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		require.NoError(t, New(db).UpdateEventTriggerStatusFrom(context.Background(), "trigger-1", domain.EventTriggerStatusWaiting, domain.EventTriggerStatusReceived, nil, nil, ""))

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "SELECT status")
			require.Len(t, args, 1)
			return &mockRow{scanFn: func(dest ...any) error {
				if queryRowErr != nil {
					return queryRowErr
				}
				*(dest[0].(*string)) = domain.EventTriggerStatusReceived
				return nil
			}}
		}

		err := New(db).UpdateEventTriggerStatusFrom(context.Background(), "trigger-1", domain.EventTriggerStatusWaiting, domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorIs(t, err, ErrEventTriggerConflict)
		require.ErrorContains(t, err, "actual received")

		queryRowErr = pgx.ErrNoRows
		err = New(db).UpdateEventTriggerStatusFrom(context.Background(), "missing", domain.EventTriggerStatusWaiting, domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorContains(t, err, "event trigger not found")

		queryRowErr = errors.New("read failed")
		err = New(db).UpdateEventTriggerStatusFrom(context.Background(), "trigger-1", domain.EventTriggerStatusWaiting, domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorContains(t, err, "read event trigger status after conflict")

		updateErr := errors.New("cas failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, updateErr
		}
		err = New(db).UpdateEventTriggerStatusFrom(context.Background(), "trigger-1", domain.EventTriggerStatusWaiting, domain.EventTriggerStatusReceived, nil, nil, "")
		require.ErrorContains(t, err, "update event trigger status from waiting")
		require.ErrorIs(t, err, updateErr)
	})

	t.Run("updates sent by and notify status", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "sent_by")
				require.Equal(t, []any{"api-key-1", "trigger-1"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		require.NoError(t, New(db).SetEventTriggerSentBy(context.Background(), "trigger-1", "api-key-1"))

		execErr := errors.New("exec failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err := New(db).SetEventTriggerSentBy(context.Background(), "trigger-1", "api-key-1")
		require.ErrorContains(t, err, "set event trigger sent_by")
		require.ErrorIs(t, err, execErr)

		db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "notify_status")
			require.Equal(t, []any{"sent", "trigger-1"}, args)
			return pgconn.NewCommandTag("UPDATE 1"), nil
		}
		require.NoError(t, New(db).UpdateEventTriggerNotifyStatus(context.Background(), "trigger-1", "sent"))

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		err = New(db).UpdateEventTriggerNotifyStatus(context.Background(), "missing", "sent")
		require.ErrorContains(t, err, "event trigger not found")

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err = New(db).UpdateEventTriggerNotifyStatus(context.Background(), "trigger-1", "sent")
		require.ErrorContains(t, err, "update event trigger notify status")
		require.ErrorIs(t, err, execErr)
	})
}

func TestEventTriggerListUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists expired triggers and maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "expires_at <= NOW()")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{eventTriggerScanFn(now, false)}}, nil
			},
		}

		got, err := New(db).ListExpiredEventTriggers(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "trigger-1", got[0].ID)
		require.Empty(t, got[0].EnvironmentID)

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list expired event triggers"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan expired event trigger"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list expired event triggers rows"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListExpiredEventTriggers(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("lists by project with every optional filter", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "environment_id = $2")
				require.Contains(t, sql, "status = $3")
				require.Contains(t, sql, "workflow_run_id = $4")
				require.Contains(t, sql, "source_type = $5")
				require.Contains(t, sql, "requested_at < $6")
				require.Contains(t, sql, "LIMIT $7")
				require.Equal(t, []any{"project-1", "env-prod", "waiting", "workflow-run-1", "workflow_step", cursor, 25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{eventTriggerScanFn(now, true)}}, nil
			},
		}

		got, err := New(db).ListEventTriggersByProject(context.Background(), "project-1", "env-prod", "waiting", "workflow-run-1", "workflow_step", 25, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "env-prod", got[0].EnvironmentID)
	})

	t.Run("lists by project maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list event triggers by project"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan event trigger"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list event triggers by project rows"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListEventTriggersByProject(context.Background(), "project-1", "", "", "", "", 10, nil)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("lists by escaped key prefix with and without project scope", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tests := []struct {
			name           string
			projectID      string
			wantArgs       []any
			wantProjectSQL bool
		}{
			{name: "global", wantArgs: []any{`acme\\\%\_.%`}},
			{name: "project", projectID: "project-1", wantArgs: []any{`acme\\\%\_.%`, "project-1"}, wantProjectSQL: true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
						require.Contains(t, sql, "event_key LIKE $1")
						if tc.wantProjectSQL {
							require.Contains(t, sql, "project_id = $2")
						} else {
							require.NotContains(t, sql, "project_id = $2")
						}
						require.Equal(t, tc.wantArgs, args)
						return &mockRows{scanFns: []func(dest ...any) error{eventTriggerScanFn(now, false)}}, nil
					},
				}

				got, err := New(db).ListEventTriggersByKeyPrefix(context.Background(), `acme\%_.`, tc.projectID)
				require.NoError(t, err)
				require.Len(t, got, 1)
			})
		}
	})

	t.Run("lists by key prefix maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list event triggers by key prefix"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan event trigger by key prefix"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list event triggers by key prefix rows"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListEventTriggersByKeyPrefix(context.Background(), "event.", "")
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("lists received triggers with stale steps", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "UNION ALL")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{eventTriggerScanFn(now, false)}}, nil
			},
		}

		got, err := New(db).ListReceivedEventTriggersWithStaleSteps(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list received event triggers with stale steps"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan stale event trigger"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list received event triggers with stale steps rows"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListReceivedEventTriggersWithStaleSteps(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestEventTriggerCancelAndRetentionUnit(t *testing.T) {
	t.Parallel()

	t.Run("cancels workflow and job triggers", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				args = append([]any(nil), gotArgs...)
				if strings.Contains(sql, "workflow_run_id") {
					return pgconn.NewCommandTag("UPDATE 3"), nil
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		affected, err := New(db).CancelEventTriggersByWorkflowRun(context.Background(), "workflow-run-1")
		require.NoError(t, err)
		require.EqualValues(t, 3, affected)
		require.Equal(t, []any{"workflow-run-1"}, args)

		require.NoError(t, New(db).CancelEventTriggerByJobRun(context.Background(), "job-run-1"))
		require.Equal(t, []any{"job-run-1"}, args)

		execErr := errors.New("cancel failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).CancelEventTriggersByWorkflowRun(context.Background(), "workflow-run-1")
		require.ErrorContains(t, err, "cancel event triggers for workflow run")
		require.ErrorIs(t, err, execErr)

		err = New(db).CancelEventTriggerByJobRun(context.Background(), "job-run-1")
		require.ErrorContains(t, err, "cancel event trigger for job run")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("counts and deletes finished triggers", func(t *testing.T) {
		t.Parallel()

		before := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "COUNT(*)")
				require.Equal(t, []any{before}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 42
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM event_triggers")
				require.Equal(t, []any{before, 100}, args)
				return pgconn.NewCommandTag("DELETE 7"), nil
			},
		}

		count, err := New(db).CountEventTriggersFinishedBefore(context.Background(), before)
		require.NoError(t, err)
		require.EqualValues(t, 42, count)

		deleted, err := New(db).DeleteEventTriggersFinishedBefore(context.Background(), before, 100)
		require.NoError(t, err)
		require.EqualValues(t, 7, deleted)

		queryErr := errors.New("count failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return queryErr }}
		}
		_, err = New(db).CountEventTriggersFinishedBefore(context.Background(), before)
		require.ErrorContains(t, err, "count old event triggers")
		require.ErrorIs(t, err, queryErr)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).DeleteEventTriggersFinishedBefore(context.Background(), before, 100)
		require.ErrorContains(t, err, "delete old event triggers")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("counts and deletes finished project triggers", func(t *testing.T) {
		t.Parallel()

		before := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "project_id = $1")
				require.Equal(t, []any{"project-1", "env-prod", before}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 12
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "project_id = $1")
				require.Equal(t, []any{"project-1", "env-prod", before, 50}, args)
				return pgconn.NewCommandTag("DELETE 5"), nil
			},
		}

		count, err := New(db).CountEventTriggersFinishedBeforeForProject(context.Background(), "project-1", "env-prod", before)
		require.NoError(t, err)
		require.EqualValues(t, 12, count)

		deleted, err := New(db).DeleteEventTriggersFinishedBeforeForProject(context.Background(), "project-1", "env-prod", before, 50)
		require.NoError(t, err)
		require.EqualValues(t, 5, deleted)

		queryErr := errors.New("count failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return queryErr }}
		}
		_, err = New(db).CountEventTriggersFinishedBeforeForProject(context.Background(), "project-1", "env-prod", before)
		require.ErrorContains(t, err, "count old project event triggers")
		require.ErrorIs(t, err, queryErr)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).DeleteEventTriggersFinishedBeforeForProject(context.Background(), "project-1", "env-prod", before, 50)
		require.ErrorContains(t, err, "delete old project event triggers")
		require.ErrorIs(t, err, execErr)
	})
}

func TestEventTriggerBatchReceiveAndHelpersUnit(t *testing.T) {
	t.Parallel()

	t.Run("receives event and requeues run in a transaction", func(t *testing.T) {
		t.Parallel()

		tx := newReceiveEventTx(t, nil, "", nil)
		err := New(&eventTriggerBeginner{tx: tx}).ReceiveEventAndRequeueRun(context.Background(), "trigger-1", nil, time.Now().UTC(), "run-1")
		require.NoError(t, err)
		require.Equal(t, 1, tx.commits)
		require.Zero(t, tx.rollbacks)
	})

	t.Run("receives event and checkpoints payload", func(t *testing.T) {
		t.Parallel()

		payload := json.RawMessage(`{"checkpoint":true}`)
		checkpointTx := receiveEventCheckpointTx(t, nil)
		tx := newReceiveEventTx(t, payload, "", nil)
		tx.beginFn = func(context.Context) (pgx.Tx, error) {
			return checkpointTx, nil
		}

		err := New(&eventTriggerBeginner{tx: tx}).ReceiveEventAndRequeueRun(context.Background(), "trigger-1", payload, time.Now().UTC(), "run-1")
		require.NoError(t, err)
		require.Equal(t, 1, tx.commits)
		require.Equal(t, 1, checkpointTx.commits)
	})

	t.Run("wraps transactional requeue failures", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			failStep   string
			wantString string
			wantIs     error
		}{
			{name: "trigger update", failStep: "trigger", wantString: "update trigger status"},
			{name: "run state error", failStep: "run-state-error", wantString: "requeue run"},
			{name: "run state conflict", failStep: "run-state-conflict", wantString: "from waiting", wantIs: ErrRunConflict},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				failErr := errors.New("forced failure")
				tx := newReceiveEventTx(t, nil, tc.failStep, failErr)
				err := New(&eventTriggerBeginner{tx: tx}).ReceiveEventAndRequeueRun(context.Background(), "trigger-1", nil, time.Now().UTC(), "run-1")
				require.ErrorContains(t, err, tc.wantString)
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
				}
				require.Zero(t, tx.commits)
				require.Equal(t, 1, tx.rollbacks)
			})
		}
	})

	t.Run("wraps checkpoint failures", func(t *testing.T) {
		t.Parallel()

		payload := json.RawMessage(`{"checkpoint":true}`)
		checkpointErr := errors.New("checkpoint failed")
		checkpointTx := receiveEventCheckpointTx(t, checkpointErr)
		tx := newReceiveEventTx(t, payload, "", nil)
		tx.beginFn = func(context.Context) (pgx.Tx, error) {
			return checkpointTx, nil
		}

		err := New(&eventTriggerBeginner{tx: tx}).ReceiveEventAndRequeueRun(context.Background(), "trigger-1", payload, time.Now().UTC(), "run-1")
		require.ErrorContains(t, err, "create event checkpoint")
		require.ErrorIs(t, err, checkpointErr)
		require.Zero(t, tx.commits)
		require.Equal(t, 1, tx.rollbacks)
		require.Zero(t, checkpointTx.commits)
		require.Equal(t, 1, checkpointTx.rollbacks)
	})

	t.Run("requires transactions when requeueing runs", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).ReceiveEventAndRequeueRun(context.Background(), "trigger-1", nil, time.Now().UTC(), "run-1")
		require.ErrorContains(t, err, "requires transaction support")
	})

	t.Run("batch receive handles empty conflict success and nonfatal sent by errors", func(t *testing.T) {
		t.Parallel()

		empty, err := New(&mockDBTX{}).BatchReceiveEventTriggers(context.Background(), nil, nil, time.Now().UTC(), "")
		require.NoError(t, err)
		require.Nil(t, empty)

		var execCalls int
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execCalls++
				if strings.Contains(sql, "sent_by") {
					require.Equal(t, []any{"api-key-1", "trigger-1"}, args)
					return pgconn.CommandTag{}, errors.New("audit failed")
				}
				switch args[4] {
				case "trigger-1":
					return pgconn.NewCommandTag("UPDATE 1"), nil
				case "trigger-2":
					return pgconn.NewCommandTag("UPDATE 0"), nil
				default:
					return pgconn.CommandTag{}, errors.New("unexpected trigger")
				}
			},
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "SELECT status")
				require.Equal(t, []any{"trigger-2"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = domain.EventTriggerStatusReceived
					return nil
				}}
			},
		}

		resolved, err := New(db).BatchReceiveEventTriggers(context.Background(), []string{"trigger-1", "trigger-2"}, json.RawMessage(`{"ok":true}`), time.Now().UTC(), "api-key-1")
		require.NoError(t, err)
		require.Equal(t, []string{"trigger-1"}, resolved)
		require.Equal(t, 3, execCalls)
	})

	t.Run("batch receive returns partial results on hard update errors", func(t *testing.T) {
		t.Parallel()

		var updateCalls int
		updateErr := errors.New("update failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				if strings.Contains(sql, "sent_by") {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
				updateCalls++
				if args[4] == "trigger-2" {
					return pgconn.CommandTag{}, updateErr
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		resolved, err := New(db).BatchReceiveEventTriggers(context.Background(), []string{"trigger-1", "trigger-2"}, nil, time.Now().UTC(), "")
		require.ErrorContains(t, err, "update trigger trigger-2")
		require.ErrorIs(t, err, updateErr)
		require.Equal(t, []string{"trigger-1"}, resolved)
		require.Equal(t, 2, updateCalls)
	})

	t.Run("scanner returns nil optional fields and scan errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		got, err := scanEventTrigger(&mockRow{scanFn: eventTriggerScanFn(now, false)})
		require.NoError(t, err)
		require.Equal(t, "trigger-1", got.ID)
		require.Empty(t, got.EnvironmentID)
		require.Empty(t, got.WorkflowRunID)
		require.Empty(t, got.RequestPayload)
		require.Nil(t, got.ReceivedAt)

		scanErr := errors.New("scan failed")
		_, err = scanEventTrigger(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("default and active count helpers", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "fallback", defaultIfEmpty("", "fallback"))
		require.Equal(t, "value", defaultIfEmpty("value", "fallback"))

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "status = 'waiting'")
				require.Equal(t, []any{"project-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 8
					return nil
				}}
			},
		}
		count, err := New(db).CountActiveEventTriggersByProject(context.Background(), "project-1")
		require.NoError(t, err)
		require.Equal(t, 8, count)

		countErr := errors.New("count failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return countErr }}
		}
		_, err = New(db).CountActiveEventTriggersByProject(context.Background(), "project-1")
		require.ErrorContains(t, err, "count active event triggers")
		require.ErrorIs(t, err, countErr)
	})
}
