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
	"github.com/stretchr/testify/require"
)

func fillAuditEventDest(dest []any, id string, createdAt time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "actor-1"
	*(dest[3].(*string)) = "user"
	*(dest[4].(*string)) = domain.AuditActionJobCreated
	*(dest[5].(*string)) = "job"
	*(dest[6].(*string)) = "job-1"
	*(dest[7].(*json.RawMessage)) = json.RawMessage(`{"ok":true}`)
	*(dest[8].(*string)) = "signature-1"
	*(dest[9].(*string)) = ZeroHash
	*(dest[10].(*time.Time)) = createdAt
	*(dest[11].(*string)) = "127.0.0.1"
	*(dest[12].(*string)) = "agent"
	*(dest[13].(*string)) = "request-1"
	*(dest[14].(*string)) = "trace-1"
	*(dest[15].(*uint16)) = 4
	*(dest[16].(*bool)) = false
	*(dest[17].(*int)) = 2
	*(dest[18].(*string)) = "job"
}

func auditEventScanFn(id string, createdAt time.Time) func(dest ...any) error {
	return func(dest ...any) error {
		fillAuditEventDest(dest, id, createdAt)
		return nil
	}
}

func TestAuditSignatureHelpersUnit(t *testing.T) {
	t.Parallel()

	t.Run("derives deterministic non-empty key", func(t *testing.T) {
		t.Parallel()

		_, err := DeriveAuditSigningKey("")
		require.ErrorContains(t, err, "secret is empty")

		keyA, err := DeriveAuditSigningKey("internal-secret")
		require.NoError(t, err)
		keyB, err := DeriveAuditSigningKey("internal-secret")
		require.NoError(t, err)
		require.Len(t, keyA, 32)
		require.Equal(t, keyA, keyB)
	})

	t.Run("canonical signature versions bind added fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
		key := []byte("01234567890123456789012345678901")
		base := &domain.AuditEvent{
			ID:            "audit-1",
			ProjectID:     "project-1",
			ActorID:       "actor-1",
			ActorType:     "user",
			Action:        domain.AuditActionJobCreated,
			ResourceType:  "job",
			ResourceID:    "job-1",
			Details:       json.RawMessage(`{"ok":true}`),
			CreatedAt:     createdAt,
			PreviousHash:  ZeroHash,
			RemoteIP:      "127.0.0.1",
			UserAgent:     "agent",
			RequestID:     "request-1",
			TraceID:       "trace-1",
			SchemaVersion: 4,
			IsAnchor:      true,
			RotationEpoch: 3,
			ShardID:       "job",
		}

		v4 := ComputeAuditSignature(base, key)
		changedShard := *base
		changedShard.ShardID = "workflow"
		require.NotEqual(t, v4, ComputeAuditSignature(&changedShard, key))

		v3 := *base
		v3.SchemaVersion = 3
		require.NotEqual(t, v4, ComputeAuditSignature(&v3, key))

		v2 := *base
		v2.SchemaVersion = 2
		v2.IsAnchor = false
		v2.RotationEpoch = 0
		require.NotEqual(t, ComputeAuditSignature(&v3, key), ComputeAuditSignature(&v2, key))

		v1 := *base
		v1.SchemaVersion = 1
		require.NotEqual(t, ComputeAuditSignature(&v2, key), ComputeAuditSignature(&v1, key))
	})

	t.Run("length-delimited canonical form avoids adjacent field ambiguity", func(t *testing.T) {
		t.Parallel()

		left := lengthDelimitedAuditCanonical("audit:test\n", []string{"ab", "c"})
		right := lengthDelimitedAuditCanonical("audit:test\n", []string{"a", "bc"})
		require.NotEqual(t, left, right)
		require.Contains(t, left, "2:ab\n1:c\n")
		require.Equal(t, 3, decimalDigitCount(100))
	})

	t.Run("retention tombstone must match first survivor chain start and shard", func(t *testing.T) {
		t.Parallel()

		ev := domain.AuditEvent{
			Action:   domain.AuditActionRetentionTrimmed,
			IsAnchor: true,
			ShardID:  "job",
			Details: json.RawMessage(`{
				"chain_start":"previous-signature",
				"first_surviving_event_id":"audit-2",
				"shard_id":"job"
			}`),
		}
		require.True(t, auditRetentionTombstoneJustifiesStart(ev, "audit-2", "previous-signature"))
		require.False(t, auditRetentionTombstoneJustifiesStart(ev, "", "previous-signature"))
		require.False(t, auditRetentionTombstoneJustifiesStart(ev, "audit-2", ""))
		require.False(t, auditRetentionTombstoneJustifiesStart(ev, "other", "previous-signature"))

		ev.ShardID = "workflow"
		require.False(t, auditRetentionTombstoneJustifiesStart(ev, "audit-2", "previous-signature"))
		ev.Details = json.RawMessage(`{`)
		require.False(t, auditRetentionTombstoneJustifiesStart(ev, "audit-2", "previous-signature"))
	})

	t.Run("epoch key lookup falls back only for epoch zero", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		q.SetAuditSigningKey([]byte("global-key"))

		got, err := q.keyForEpoch(map[int][]byte{1: []byte("epoch-one")}, 1)
		require.NoError(t, err)
		require.Equal(t, []byte("epoch-one"), got)

		got, err = q.keyForEpoch(map[int][]byte{0: nil}, 0)
		require.NoError(t, err)
		require.Equal(t, []byte("global-key"), got)

		_, err = q.keyForEpoch(map[int][]byte{2: nil}, 2)
		require.ErrorContains(t, err, "no stored key for epoch 2")
		_, err = q.keyForEpoch(map[int][]byte{}, 3)
		require.ErrorContains(t, err, "no stored key for epoch 3")
	})

	t.Run("signature comparison accepts legacy epoch zero fallback", func(t *testing.T) {
		t.Parallel()

		legacyKey := []byte("legacy-key")
		epochKey := []byte("epoch-key")
		ev := &domain.AuditEvent{
			ID:            "audit-1",
			ProjectID:     "project-1",
			CreatedAt:     time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
			PreviousHash:  ZeroHash,
			SchemaVersion: 4,
			RotationEpoch: 0,
		}
		ev.Signature = ComputeAuditSignature(ev, legacyKey)

		q := New(&mockDBTX{})
		q.SetAuditSigningKey(legacyKey)
		require.True(t, q.auditSignatureMatchesEpoch(ev, epochKey))

		ev.RotationEpoch = 1
		require.False(t, q.auditSignatureMatchesEpoch(ev, epochKey))
	})
}

func TestCreateAuditEventUnit(t *testing.T) {
	t.Parallel()

	t.Run("rejects nil event", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).CreateAuditEvent(context.Background(), nil)
		require.ErrorContains(t, err, "event is nil")
	})

	t.Run("defaults details schema shard and previous hash before insert", func(t *testing.T) {
		t.Parallel()

		var insertArgs []any
		var lockKeys []string
		queryRowCalls := 0
		hookCalled := false
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRowCalls++
				switch {
				case strings.Contains(sql, "pg_try_advisory_xact_lock"):
					require.Len(t, args, 2)
					lockKeys = append(lockKeys, args[0].(string)+args[1].(string))
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = true
						return nil
					}}
				case strings.Contains(sql, "MAX(rotation_epoch)"):
					require.Equal(t, []any{"project-1", "job", ZeroHash}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*int)) = 2
						*(dest[1].(*string)) = "previous-signature"
						return nil
					}}
				case strings.Contains(sql, "INSERT INTO audit_events"):
					insertArgs = append([]any(nil), args...)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*json.RawMessage)) = json.RawMessage(`{"canonical":true}`)
						return nil
					}}
				default:
					require.Failf(t, "unexpected query", "sql=%s args=%v", sql, args)
					return &mockRow{}
				}
			},
		}
		q := New(db)
		q.auditEventPostInsertHook = func(context.Context) error {
			hookCalled = true
			return nil
		}
		ev := &domain.AuditEvent{
			ProjectID:    "project-1",
			ActorID:      "actor-1",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job-1",
		}

		require.NoError(t, q.CreateAuditEvent(context.Background(), ev))
		require.NotEmpty(t, ev.ID)
		require.Equal(t, "job", ev.ShardID)
		require.Equal(t, uint16(4), ev.SchemaVersion)
		require.Equal(t, 2, ev.RotationEpoch)
		require.Equal(t, "previous-signature", ev.PreviousHash)
		require.JSONEq(t, `{"canonical":true}`, string(ev.Details))
		require.True(t, hookCalled)
		require.Equal(t, []string{
			AdvisoryLockNsAuditRotate + "project-1",
			AdvisoryLockNsAuditChainShard + "project-1:job",
		}, lockKeys)
		require.Len(t, insertArgs, 18)
		require.JSONEq(t, `{}`, string(insertArgs[7].(json.RawMessage)))
		require.Equal(t, "previous-signature", insertArgs[8])
		require.Equal(t, uint16(4), insertArgs[14])
		require.Equal(t, "job", insertArgs[17])
		require.Equal(t, 4, queryRowCalls)
	})

	t.Run("wraps lock read insert and hook errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			failAtCall int
			hookErr    error
			want       string
		}{
			{name: "rotation lock", failAtCall: 1, want: "rotation lock"},
			{name: "chain lock", failAtCall: 2, want: "chain lock"},
			{name: "read previous hash", failAtCall: 3, want: "read epoch and prev hash"},
			{name: "insert", failAtCall: 4, want: "insert"},
			{name: "hook", hookErr: errors.New("hook failed"), want: "post-insert hook"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				calls := 0
				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
						calls++
						if tt.failAtCall == calls {
							return &mockRow{scanFn: func(...any) error { return errors.New("query failed") }}
						}
						if strings.Contains(sql, "pg_try_advisory_xact_lock") {
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*bool)) = true
								return nil
							}}
						}
						if strings.Contains(sql, "MAX(rotation_epoch)") {
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*int)) = 0
								*(dest[1].(*string)) = ZeroHash
								return nil
							}}
						}
						return &mockRow{scanFn: func(dest ...any) error {
							*(dest[0].(*json.RawMessage)) = json.RawMessage(`{}`)
							return nil
						}}
					},
				}
				q := New(db)
				if tt.hookErr != nil {
					q.auditEventPostInsertHook = func(context.Context) error { return tt.hookErr }
				}
				err := q.CreateAuditEvent(context.Background(), &domain.AuditEvent{
					ProjectID:    "project-1",
					ResourceType: "job",
				})
				require.ErrorContains(t, err, tt.want)
			})
		}
	})
}

func TestAuditEventListGetAndStreamUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists with all filters ascending", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(-time.Minute)
		from := now.Add(-time.Hour)
		to := now
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "actor_id = $2")
				require.Contains(t, sql, "resource_type = $3")
				require.Contains(t, sql, "resource_id = $4")
				require.Contains(t, sql, "created_at > $5")
				require.Contains(t, sql, "created_at >= $6")
				require.Contains(t, sql, "created_at <= $7")
				require.Contains(t, sql, "ORDER BY created_at ASC, id ASC LIMIT $8")
				require.Equal(t, []any{"project-1", "actor-1", "job", "job-1", cursor, from, to, 25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{auditEventScanFn("audit-1", now)}}, nil
			},
		}

		got, err := New(db).ListAuditEvents(context.Background(), "project-1", "actor-1", "job", "job-1", 25, &cursor, &from, &to, true)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "audit-1", got[0].ID)
		require.Equal(t, uint16(4), got[0].SchemaVersion)
		require.Equal(t, "job", got[0].ShardID)
	})

	t.Run("list defaults to descending cursor and wraps row failures", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list audit events"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan audit event"},
			{name: "rows", rowsErr: errors.New("rows failed"), wantString: "rows failed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				cursor := time.Now().UTC()
				db := &mockDBTX{
					queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
						require.Contains(t, sql, "created_at < $2")
						require.Contains(t, sql, "ORDER BY created_at DESC, id DESC")
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
				_, err := New(db).ListAuditEvents(context.Background(), "project-1", "", "", "", 10, &cursor, nil, nil, false)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("gets event and maps not found", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE id = $1 AND project_id = $2")
				require.Equal(t, []any{"audit-1", "project-1"}, args)
				return &mockRow{scanFn: auditEventScanFn("audit-1", now)}
			},
		}
		got, err := New(db).GetAuditEvent(context.Background(), "project-1", "audit-1")
		require.NoError(t, err)
		require.Equal(t, "audit-1", got.ID)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).GetAuditEvent(context.Background(), "project-1", "missing")
		require.ErrorIs(t, err, ErrAuditEventNotFound)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).GetAuditEvent(context.Background(), "project-1", "audit-1")
		require.ErrorContains(t, err, "get audit event")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("streams with filters and stops on callback error", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		from := now.Add(-time.Hour)
		to := now
		callbackErr := errors.New("callback stopped")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "actor_id = $2")
				require.Contains(t, sql, "resource_type = $3")
				require.Contains(t, sql, "created_at >= $4")
				require.Contains(t, sql, "created_at <= $5")
				require.Contains(t, sql, "ORDER BY created_at ASC")
				require.Equal(t, []any{"project-1", "actor-1", "job", from, to}, args)
				return &mockRows{scanFns: []func(dest ...any) error{auditEventScanFn("audit-1", now)}}, nil
			},
		}
		err := New(db).StreamAuditEvents(context.Background(), "project-1", "actor-1", "job", from, to, func(ev *domain.AuditEvent) error {
			require.Equal(t, "audit-1", ev.ID)
			return callbackErr
		})
		require.ErrorIs(t, err, callbackErr)
	})

	t.Run("stream wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "stream audit events"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan audit event"},
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
				err := New(db).StreamAuditEvents(context.Background(), "project-1", "", "", time.Now().Add(-time.Hour), time.Now(), func(*domain.AuditEvent) error {
					return nil
				})
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})
}
