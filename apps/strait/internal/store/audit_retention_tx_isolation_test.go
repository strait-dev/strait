package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeIsolationRow is the minimal pgx.Row implementation used by the isolation
// plumbing test — it is never actually scanned because fakeTxBeginner.
// BeginTx returns an error and WithTxOptions aborts before the first
// Query.
type fakeIsolationRow struct{}

func (fakeIsolationRow) Scan(_ ...any) error { return errors.New("fakeIsolationRow: not implemented") }

// fakeTxBeginnerOptions captures the options passed to BeginTx. It
// deliberately errors out so WithTxOptions returns immediately — we
// only care about what isolation level the caller asked for.
type fakeTxBeginnerOptions struct {
	capturedOpts pgx.TxOptions
	called       bool
}

func (f *fakeTxBeginnerOptions) BeginTx(_ context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	f.capturedOpts = opts
	f.called = true
	return nil, errors.New("begin tx: fake does not open real tx")
}

// Exec/Query/QueryRow satisfy the DBTX interface the Queries struct
// expects for non-tx code paths; they are never exercised here.
func (f *fakeTxBeginnerOptions) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("fake: exec not supported")
}

func (f *fakeTxBeginnerOptions) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("fake: query not supported")
}

func (f *fakeTxBeginnerOptions) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return fakeIsolationRow{}
}

// TestDeleteAuditEventsBeforeExcluding_RepeatableRead_IsolationLevel verifies
// the retention-reaper bulk trim requests pgx.RepeatableRead when opening
// its transaction. Without this the DISTINCT project-id SELECT and the
// per-project DELETEs can race with concurrent writers under the default
// READ COMMITTED — the tx could trim a freshly-inserted row on a project
// the SELECT never saw, and no tombstone would be emitted for it.
func TestDeleteAuditEventsBeforeExcluding_RepeatableRead_IsolationLevel(t *testing.T) {
	t.Parallel()

	fake := &fakeTxBeginnerOptions{}
	q := New(fake)

	_, err := q.DeleteAuditEventsBeforeExcluding(context.Background(), time.Now().UTC(), nil)
	// We expect the BeginTx error to bubble up — that is proof the
	// function requested a tx; the opts it requested are what we assert.
	if err == nil {
		t.Fatal("expected BeginTx error from fake, got nil")
	}
	if !fake.called {
		t.Fatal("fake BeginTx was never called; the retention trim did not open a tx")
	}
	if fake.capturedOpts.IsoLevel != pgx.RepeatableRead {
		t.Errorf("IsoLevel = %q, want %q", fake.capturedOpts.IsoLevel, pgx.RepeatableRead)
	}
}
