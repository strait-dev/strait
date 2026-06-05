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
)

type mockRows struct {
	scanFns []func(dest ...any) error
	idx     int
	err     error
}

func (m *mockRows) Close() {}

func (m *mockRows) Err() error { return m.err }

func (m *mockRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (m *mockRows) Next() bool {
	if m.idx >= len(m.scanFns) {
		return false
	}
	m.idx++
	return true
}

func (m *mockRows) Scan(dest ...any) error {
	if m.idx == 0 || m.idx > len(m.scanFns) {
		return errors.New("scan called without next")
	}
	return m.scanFns[m.idx-1](dest...)
}

func (m *mockRows) Values() ([]any, error) { return nil, nil }

func (m *mockRows) RawValues() [][]byte { return nil }

func (m *mockRows) Conn() *pgx.Conn { return nil }

func TestGetDebugBundle(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	db := &mockDBTX{}

	db.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
		if !strings.Contains(sql, "FROM job_runs") {
			t.Fatalf("unexpected queryRow SQL: %s", sql)
		}
		// GetDebugBundle now probes visibility (visible_until) before
		// fetching the full run row; serve a "visible" answer here so
		// the bundle proceeds to its existing happy-path fetch.
		if strings.Contains(sql, "visible_until") {
			return &mockRow{scanFn: func(dest ...any) error {
				if len(dest) != 1 {
					t.Fatalf("visible_until probe: unexpected scan dest count: %d", len(dest))
				}
				*dest[0].(**time.Time) = nil
				return nil
			}}
		}
		return &mockRow{scanFn: func(dest ...any) error {
			if len(dest) != 35 {
				t.Fatalf("unexpected scan dest count: %d", len(dest))
			}
			*dest[0].(*string) = "run-1"
			*dest[1].(*string) = "job-1"
			*dest[2].(*string) = "proj-1"
			*dest[3].(*domain.RunStatus) = domain.StatusCompleted
			*dest[4].(*int) = 1
			*dest[5].(*[]byte) = json.RawMessage(`{"input":true}`)
			*dest[6].(*[]byte) = json.RawMessage(`{"ok":true}`)
			*dest[7].(*[]byte) = json.RawMessage(`{"m":"v"}`)
			*dest[10].(*string) = domain.TriggerManual
			*dest[18].(*int) = 5
			*dest[20].(*int) = 2
			*dest[21].(*time.Time) = now
			*dest[24].(*bool) = true
			*dest[26].(*int) = 0
			// dest[26] = tags, dest[27] = job_version_id, dest[28] = created_by, dest[29] = batch_id, dest[30] = concurrency_key, dest[31] = execution_mode
			return nil
		}}
	}

	db.queryFn = func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
		switch {
		case strings.Contains(sql, "FROM run_events"):
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					*dest[0].(*string) = "evt-1"
					*dest[1].(*string) = "run-1"
					*dest[2].(*domain.EventType) = domain.EventLog
					level := "info"
					*dest[3].(**string) = &level
					*dest[4].(*string) = "hello"
					*dest[5].(*[]byte) = json.RawMessage(`{"k":"v"}`)
					*dest[6].(*time.Time) = now
					return nil
				},
			}}, nil
		case strings.Contains(sql, "FROM run_checkpoints"):
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					*dest[0].(*string) = "cp-1"
					*dest[1].(*string) = "run-1"
					*dest[2].(*int) = 1
					*dest[3].(*string) = "sdk"
					*dest[4].(*[]byte) = json.RawMessage(`{"step":1}`)
					*dest[5].(*time.Time) = now
					return nil
				},
			}}, nil
		case strings.Contains(sql, "FROM run_outputs"):
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					*dest[0].(*string) = "out-1"
					*dest[1].(*string) = "run-1"
					*dest[2].(*string) = "result"
					*dest[3].(*[]byte) = json.RawMessage(`{"type":"object"}`)
					*dest[4].(*[]byte) = json.RawMessage(`{"done":true}`)
					*dest[5].(*time.Time) = now
					return nil
				},
			}}, nil
		case strings.Contains(sql, "FROM run_resource_snapshots"):
			return &mockRows{scanFns: nil}, nil
		default:
			t.Fatalf("unexpected query SQL: %s", sql)
			return nil, nil
		}
	}

	q := New(db)
	bundle, err := q.GetDebugBundle(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetDebugBundle() error = %v", err)
	}
	if bundle == nil || bundle.Run == nil {
		t.Fatal("expected non-nil bundle and run")
	}
	if !bundle.Run.DebugMode {
		t.Fatal("expected debug mode to be true")
	}
	if len(bundle.Events) != 1 || len(bundle.Checkpoints) != 1 || len(bundle.Outputs) != 1 {
		t.Fatalf("unexpected bundle lengths: events=%d checkpoints=%d outputs=%d",
			len(bundle.Events), len(bundle.Checkpoints), len(bundle.Outputs))
	}
}

func TestUpdateRunDebugMode(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "UPDATE job_runs") || !strings.Contains(sql, "debug_mode IS DISTINCT FROM") {
				t.Fatalf("unexpected SQL: %s", sql)
			}
			if len(args) != 2 {
				t.Fatalf("unexpected arg count: %d", len(args))
			}
			if args[0] != true || args[1] != "run-1" {
				t.Fatalf("unexpected args: %#v", args)
			}
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				*dest[1].(*bool) = true
				return nil
			}}
		}}

		q := New(db)
		if err := q.UpdateRunDebugMode(context.Background(), "run-1", true); err != nil {
			t.Fatalf("UpdateRunDebugMode() error = %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				*dest[1].(*bool) = false
				return nil
			}}
		}}

		q := New(db)
		err := q.UpdateRunDebugMode(context.Background(), "missing-run", false)
		if !errors.Is(err, ErrRunNotFound) {
			t.Fatalf("expected ErrRunNotFound, got %v", err)
		}
	})

	t.Run("no-op existing run", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				*dest[1].(*bool) = false
				return nil
			}}
		}}

		q := New(db)
		if err := q.UpdateRunDebugMode(context.Background(), "run-1", true); err != nil {
			t.Fatalf("UpdateRunDebugMode() error = %v", err)
		}
	})
}

// Regression: GetDebugBundle must return ErrRunNotFound when the run
// has been masked via visible_until <= NOW(). The DLQ age-out flow
// uses this column to take rich-PII rows out of circulation; the
// debug-bundle endpoint must not undo that decision.
func TestGetDebugBundle_MaskedRun_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	masked := time.Now().Add(-time.Minute)
	db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
		if !strings.Contains(sql, "visible_until") {
			t.Fatalf("expected visibility probe first, got: %s", sql)
		}
		return &mockRow{scanFn: func(dest ...any) error {
			*dest[0].(**time.Time) = &masked
			return nil
		}}
	}}
	q := New(db)
	bundle, err := q.GetDebugBundle(context.Background(), "run-1")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for masked run, got bundle=%v err=%v", bundle, err)
	}
}
