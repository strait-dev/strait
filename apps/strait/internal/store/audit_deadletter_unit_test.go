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

func fillAuditDeadletterDest(dest []any, id string, createdAt time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "actor-1"
	*(dest[3].(*string)) = "user"
	*(dest[4].(*string)) = domain.AuditActionJobCreated
	*(dest[5].(*string)) = "job"
	*(dest[6].(*string)) = "job-1"
	*(dest[7].(*json.RawMessage)) = json.RawMessage(`{"ok":true}`)
	*(dest[8].(*time.Time)) = createdAt
	*(dest[9].(*string)) = "127.0.0.1"
	*(dest[10].(*string)) = "agent"
	*(dest[11].(*string)) = "request-1"
	*(dest[12].(*string)) = "trace-1"
	*(dest[13].(*uint16)) = 4
	if len(dest) == 15 {
		*(dest[14].(*time.Time)) = createdAt.Add(time.Minute)
	}
	if len(dest) > 15 {
		reclaimedID := "audit-reclaimed"
		*(dest[14].(*int)) = 3
		*(dest[15].(**string)) = &reclaimedID
	}
}

func auditDeadletterScanFn(id string, createdAt time.Time) func(dest ...any) error {
	return func(dest ...any) error {
		fillAuditDeadletterDest(dest, id, createdAt)
		return nil
	}
}

type auditDeadletterTx struct {
	*fakeTx
	commitErr error
	commits   int
	rollbacks int
}

func (tx *auditDeadletterTx) Commit(context.Context) error {
	tx.commits++
	return tx.commitErr
}

func (tx *auditDeadletterTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type auditDeadletterBeginner struct {
	mockDBTX
	tx       *auditDeadletterTx
	beginErr error
}

func (b *auditDeadletterBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func auditDeadletterReplayTx(t *testing.T, markTag, deleteTag pgconn.CommandTag) *auditDeadletterTx {
	t.Helper()

	queryRowCalls := 0
	execCalls := 0
	tx := &auditDeadletterTx{}
	tx.fakeTx = &fakeTx{
		beginFn: func(context.Context) (pgx.Tx, error) {
			return tx, nil
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			queryRowCalls++
			switch {
			case strings.Contains(sql, "FROM audit_events_deadletter") && strings.Contains(sql, "FOR UPDATE"):
				require.Equal(t, []any{"dlq-1", "project-1"}, args)
				return &mockRow{scanFn: auditDeadletterScanFn("dlq-1", time.Now().UTC())}
			case strings.Contains(sql, "pg_try_advisory_xact_lock"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			case strings.Contains(sql, "MAX(rotation_epoch)"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 0
					*(dest[1].(*string)) = ZeroHash
					return nil
				}}
			case strings.Contains(sql, "INSERT INTO audit_events"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*json.RawMessage)) = json.RawMessage(`{"ok":true}`)
					return nil
				}}
			default:
				require.Failf(t, "unexpected query row", "call=%d sql=%s args=%v", queryRowCalls, sql, args)
				return &mockRow{}
			}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls++
			switch {
			case strings.Contains(sql, "SET reclaimed_event_id"):
				require.Equal(t, []any{"dlq-1", "project-1", "audit-new"}, args)
				return markTag, nil
			case strings.Contains(sql, "DELETE FROM audit_events_deadletter"):
				require.Equal(t, []any{"dlq-1", "project-1"}, args)
				return deleteTag, nil
			default:
				require.Failf(t, "unexpected exec", "call=%d sql=%s args=%v", execCalls, sql, args)
				return pgconn.CommandTag{}, nil
			}
		},
	}
	return tx
}

func TestAuditDeadletterCreateCountAndListUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates with generated defaults", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO audit_events_deadletter")
				args = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		ev := &domain.AuditEvent{
			ProjectID:    "project-1",
			ActorID:      "actor-1",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job-1",
		}

		require.NoError(t, New(db).CreateAuditEventDeadletter(context.Background(), ev, "db down", 2))
		require.NotEmpty(t, ev.ID)
		require.False(t, ev.CreatedAt.IsZero())
		require.Equal(t, domain.AuditEventSchemaVersionCurrent, ev.SchemaVersion)
		require.JSONEq(t, `{}`, string(args[7].(json.RawMessage)))
		require.Equal(t, "db down", args[9])
		require.Equal(t, 2, args[10])
	})

	t.Run("wraps create and count errors", func(t *testing.T) {
		t.Parallel()

		execErr := errors.New("insert failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			},
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("count failed") }}
			},
		}
		err := New(db).CreateAuditEventDeadletter(context.Background(), &domain.AuditEvent{}, "down", 0)
		require.ErrorContains(t, err, "create audit event deadletter")
		require.ErrorIs(t, err, execErr)

		_, err = New(db).CountAuditEventsDeadletter(context.Background())
		require.ErrorContains(t, err, "count audit deadletter")
	})

	t.Run("counts and lists oldest rows", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "COUNT(*)")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 7
					return nil
				}}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY queued_at ASC")
				require.Equal(t, []any{25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{auditDeadletterScanFn("dlq-1", now)}}, nil
			},
		}

		count, err := New(db).CountAuditEventsDeadletter(context.Background())
		require.NoError(t, err)
		require.Equal(t, int64(7), count)

		events, ids, err := New(db).ListAuditEventsDeadletter(context.Background(), 25)
		require.NoError(t, err)
		require.Equal(t, []string{"dlq-1"}, ids)
		require.Equal(t, "project-1", events[0].ProjectID)
	})

	t.Run("list maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list audit deadletter"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan audit deadletter"},
			{name: "rows", rowsErr: errors.New("rows failed"), wantString: "rows failed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						rows := &mockRows{err: tt.rowsErr}
						if tt.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tt.scanErr }}
						}
						return rows, nil
					},
				}
				_, _, err := New(db).ListAuditEventsDeadletter(context.Background(), 10)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})
}

func TestAuditDeadletterByProjectAndGetUnit(t *testing.T) {
	t.Parallel()

	t.Run("cursor round trips composite and legacy forms", func(t *testing.T) {
		t.Parallel()

		queuedAt := time.Date(2026, 6, 19, 12, 0, 0, 123, time.UTC)
		cursor := auditDeadletterCursor(queuedAt, "dlq-1")
		gotTime, gotID, err := parseAuditDeadletterCursor(cursor)
		require.NoError(t, err)
		require.Equal(t, queuedAt, gotTime)
		require.Equal(t, "dlq-1", gotID)

		gotTime, gotID, err = parseAuditDeadletterCursor(queuedAt.Format(time.RFC3339Nano))
		require.NoError(t, err)
		require.Equal(t, queuedAt, gotTime)
		require.Empty(t, gotID)

		_, _, err = parseAuditDeadletterCursor("not-a-time")
		require.Error(t, err)
	})

	t.Run("lists by project with default limit and composite cursor", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := auditDeadletterCursor(now.Add(-time.Minute), "dlq-0")
		queries := 0
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				queries++
				if queries == 1 {
					require.Contains(t, sql, "LIMIT $2")
					require.Equal(t, []any{"project-1", 50}, args)
				} else {
					require.Contains(t, sql, "queued_at > $2 OR (queued_at = $2 AND id > $3)")
					require.Len(t, args, 4)
					require.Equal(t, "project-1", args[0])
					require.Equal(t, "dlq-0", args[2])
					require.Equal(t, 10, args[3])
				}
				return &mockRows{scanFns: []func(dest ...any) error{auditDeadletterScanFn("dlq-1", now)}}, nil
			},
		}

		events, ids, cursors, err := New(db).ListAuditEventsDeadletterByProject(context.Background(), "project-1", 0, "")
		require.NoError(t, err)
		require.Equal(t, []string{"dlq-1"}, ids)
		require.Len(t, cursors, 1)
		require.Equal(t, "dlq-1", events[0].ID)

		_, _, _, err = New(db).ListAuditEventsDeadletterByProject(context.Background(), "project-1", 10, cursor)
		require.NoError(t, err)
	})

	t.Run("by project validates input and maps failures", func(t *testing.T) {
		t.Parallel()

		_, _, _, err := New(&mockDBTX{}).ListAuditEventsDeadletterByProject(context.Background(), "", 10, "")
		require.ErrorContains(t, err, "project_id is required")
		_, _, _, err = New(&mockDBTX{}).ListAuditEventsDeadletterByProject(context.Background(), "project-1", 10, "bad-cursor")
		require.ErrorContains(t, err, "invalid cursor")

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list audit deadletter by project"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan audit deadletter row"},
			{name: "rows", rowsErr: errors.New("rows failed"), wantString: "rows failed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						rows := &mockRows{err: tt.rowsErr}
						if tt.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tt.scanErr }}
						}
						return rows, nil
					},
				}
				_, _, _, err := New(db).ListAuditEventsDeadletterByProject(context.Background(), "project-1", 10, "")
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("gets scoped row and maps missing row to nil", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "reclaimed_event_id IS NULL")
				require.Equal(t, []any{"dlq-1", "project-1"}, args)
				return &mockRow{scanFn: auditDeadletterScanFn("dlq-1", now)}
			},
		}
		got, err := New(db).GetAuditEventDeadletter(context.Background(), "dlq-1", "project-1")
		require.NoError(t, err)
		require.Equal(t, "dlq-1", got.ID)

		_, err = New(db).GetAuditEventDeadletter(context.Background(), "", "project-1")
		require.ErrorContains(t, err, "id and project_id are required")

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		got, err = New(db).GetAuditEventDeadletter(context.Background(), "missing", "project-1")
		require.NoError(t, err)
		require.Nil(t, got)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).GetAuditEventDeadletter(context.Background(), "dlq-1", "project-1")
		require.ErrorContains(t, err, "get audit deadletter")
		require.ErrorIs(t, err, scanErr)
	})
}

func TestAuditDeadletterReplayDropAndDeleteUnit(t *testing.T) {
	t.Parallel()

	t.Run("replays row inside transaction", func(t *testing.T) {
		t.Parallel()

		tx := auditDeadletterReplayTx(t, pgconn.NewCommandTag("UPDATE 1"), pgconn.NewCommandTag("DELETE 1"))
		got, replayed, err := New(&auditDeadletterBeginner{tx: tx}).ReplayAuditEventDeadletter(context.Background(), "dlq-1", "project-1", "audit-new")
		require.NoError(t, err)
		require.True(t, replayed)
		require.Equal(t, "audit-new", got.ID)
		require.Equal(t, 2, tx.commits)
		require.Equal(t, 1, tx.rollbacks)
	})

	t.Run("replay validates transaction and row states", func(t *testing.T) {
		t.Parallel()

		_, _, err := New(&mockDBTX{}).ReplayAuditEventDeadletter(context.Background(), "", "project-1", "audit-new")
		require.ErrorContains(t, err, "new_event_id are required")
		_, _, err = New(&mockDBTX{}).ReplayAuditEventDeadletter(context.Background(), "dlq-1", "project-1", "audit-new")
		require.ErrorContains(t, err, "db does not support transactions")
		_, _, err = New(&auditDeadletterBeginner{beginErr: errors.New("begin failed")}).ReplayAuditEventDeadletter(context.Background(), "dlq-1", "project-1", "audit-new")
		require.ErrorContains(t, err, "begin tx")

		tests := []struct {
			name       string
			markTag    pgconn.CommandTag
			deleteTag  pgconn.CommandTag
			commitErr  error
			wantString string
		}{
			{name: "lost after lock", markTag: pgconn.NewCommandTag("UPDATE 0"), deleteTag: pgconn.NewCommandTag("DELETE 1"), wantString: "lost deadletter row"},
			{name: "delete no row", markTag: pgconn.NewCommandTag("UPDATE 1"), deleteTag: pgconn.NewCommandTag("DELETE 0"), wantString: "delete matched no row"},
			{name: "commit", markTag: pgconn.NewCommandTag("UPDATE 1"), deleteTag: pgconn.NewCommandTag("DELETE 1"), commitErr: errors.New("commit failed"), wantString: "commit transaction"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tx := auditDeadletterReplayTx(t, tt.markTag, tt.deleteTag)
				tx.commitErr = tt.commitErr
				_, _, err := New(&auditDeadletterBeginner{tx: tx}).ReplayAuditEventDeadletter(context.Background(), "dlq-1", "project-1", "audit-new")
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("deletes scoped row", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM audit_events_deadletter")
				require.Equal(t, []any{"dlq-1", "project-1"}, args)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		require.NoError(t, New(db).DeleteAuditEventDeadletter(context.Background(), "dlq-1", "project-1"))

		err := New(db).DeleteAuditEventDeadletter(context.Background(), "", "project-1")
		require.ErrorContains(t, err, "id and project_id are required")

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		}
		err = New(db).DeleteAuditEventDeadletter(context.Background(), "missing", "project-1")
		require.ErrorContains(t, err, "no row matched")
	})

	t.Run("drops with audit inside ambient transaction", func(t *testing.T) {
		t.Parallel()

		deleteCalls := 0
		tx := &auditDeadletterTx{}
		tx.fakeTx = &fakeTx{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "SELECT id") && strings.Contains(sql, "audit_events_deadletter"):
					require.Equal(t, []any{"dlq-1", "project-1"}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "dlq-1"
						return nil
					}}
				case strings.Contains(sql, "pg_try_advisory_xact_lock"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = true
						return nil
					}}
				case strings.Contains(sql, "MAX(rotation_epoch)"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*int)) = 0
						*(dest[1].(*string)) = ZeroHash
						return nil
					}}
				case strings.Contains(sql, "INSERT INTO audit_events"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*json.RawMessage)) = json.RawMessage(`{"drop":true}`)
						return nil
					}}
				default:
					require.Failf(t, "unexpected query row", "sql=%s args=%v", sql, args)
					return &mockRow{}
				}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM audit_events_deadletter")
				require.Equal(t, []any{"dlq-1", "project-1"}, args)
				deleteCalls++
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		auditEvent := &domain.AuditEvent{
			ActorID:      "operator-1",
			ActorType:    "user",
			Action:       domain.AuditActionDeadletterDropped,
			ResourceType: "audit_events_deadletter",
			ResourceID:   "dlq-1",
			Details:      json.RawMessage(`{"drop":true}`),
		}

		dropped, err := New(&mockDBTX{}).DropAuditEventDeadletterWithAudit(ContextWithTx(context.Background(), tx), "dlq-1", "project-1", auditEvent)
		require.NoError(t, err)
		require.True(t, dropped)
		require.Equal(t, "project-1", auditEvent.ProjectID)
		require.Equal(t, 1, deleteCalls)
	})

	t.Run("drop validates input and missing row", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		_, err := q.DropAuditEventDeadletterWithAudit(context.Background(), "", "project-1", &domain.AuditEvent{})
		require.ErrorContains(t, err, "id and project_id are required")
		_, err = q.DropAuditEventDeadletterWithAudit(context.Background(), "dlq-1", "project-1", nil)
		require.ErrorContains(t, err, "audit event is required")
		_, err = q.DropAuditEventDeadletterWithAudit(context.Background(), "dlq-1", "project-1", &domain.AuditEvent{ProjectID: "other"})
		require.ErrorContains(t, err, "does not match")
		_, err = q.DropAuditEventDeadletterWithAudit(context.Background(), "dlq-1", "project-1", &domain.AuditEvent{})
		require.ErrorContains(t, err, "db does not support transactions")

		tx := &fakeTx{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		dropped, err := q.DropAuditEventDeadletterWithAudit(ContextWithTx(context.Background(), tx), "dlq-1", "project-1", &domain.AuditEvent{})
		require.NoError(t, err)
		require.False(t, dropped)
	})
}

func TestAuditDeadletterAttemptsAndRetentionUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists with attempts and reclaimed ids", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "attempt_count")
				require.Equal(t, []any{10}, args)
				return &mockRows{scanFns: []func(dest ...any) error{auditDeadletterScanFn("dlq-1", now)}}, nil
			},
		}

		events, ids, info, err := New(db).ListAuditEventsDeadletterWithAttempts(context.Background(), 10)
		require.NoError(t, err)
		require.Equal(t, []string{"dlq-1"}, ids)
		require.Equal(t, "dlq-1", events[0].ID)
		require.Equal(t, 3, info[0].AttemptCount)
		require.Equal(t, "audit-reclaimed", *info[0].ReclaimedEventID)
	})

	t.Run("attempt list maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list audit deadletter with attempts"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan audit deadletter row"},
			{name: "rows", rowsErr: errors.New("rows failed"), wantString: "rows failed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						rows := &mockRows{err: tt.rowsErr}
						if tt.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tt.scanErr }}
						}
						return rows, nil
					},
				}
				_, _, _, err := New(db).ListAuditEventsDeadletterWithAttempts(context.Background(), 10)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("increments and marks reclaimed with row-count guard", func(t *testing.T) {
		t.Parallel()

		execCalls := 0
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execCalls++
				if execCalls == 1 {
					require.Contains(t, sql, "attempt_count = attempt_count + 1")
					require.Equal(t, []any{"dlq-1"}, args)
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
				require.Contains(t, sql, "SET reclaimed_event_id")
				require.Equal(t, []any{"dlq-1", "audit-new"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		q := New(db)
		require.NoError(t, q.IncrementAuditDeadletterAttempt(context.Background(), "dlq-1"))
		require.NoError(t, q.MarkAuditDeadletterReclaimed(context.Background(), "dlq-1", "audit-new"))

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		err := q.MarkAuditDeadletterReclaimed(context.Background(), "missing", "audit-new")
		require.ErrorContains(t, err, "no row matched")
	})

	t.Run("deletes old deadletters grouped by project", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at >= TIMESTAMPTZ '2000-01-01'")
				require.Equal(t, []any{cutoff, 1000}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "project-1"
						*(dest[1].(*int64)) = 2
						return nil
					},
				}}, nil
			},
		}
		got, err := New(db).DeleteAuditDeadletterOlderThan(context.Background(), cutoff)
		require.NoError(t, err)
		require.Equal(t, map[string]int64{"project-1": 2}, got)
	})
}
