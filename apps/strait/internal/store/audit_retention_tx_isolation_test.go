package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRetentionRow / fakeRetentionDB exists to capture the surface contract
// of DeleteAuditEventsBeforeExcluding without spinning up a real Postgres:
// the helper MUST enumerate distinct project ids in autocommit (i.e. via
// q.db.Query, not after BeginTx) so that fleet-wide retention does not hold
// any advisory locks or write locks across the entire sweep. A regression
// that wraps the enumeration in a tx — e.g. by reintroducing
// withTxInheritKeys around the SELECT — would be caught here.
type fakeRetentionRow struct{}

func (fakeRetentionRow) Scan(_ ...any) error {
	return errors.New("fakeRetentionRow: not implemented")
}

type fakeRetentionDB struct {
	queryCalled bool
	beginCalled bool
}

func (f *fakeRetentionDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("fakeRetentionDB: exec not supported")
}

func (f *fakeRetentionDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	f.queryCalled = true
	return nil, errors.New("fakeRetentionDB: query not supported")
}

func (f *fakeRetentionDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return fakeRetentionRow{}
}

func (f *fakeRetentionDB) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	f.beginCalled = true
	return nil, errors.New("fakeRetentionDB: begin not supported")
}

// TestDeleteAuditEventsBeforeExcluding_EnumeratesInAutocommit asserts the
// fleet-wide retention helper enumerates projects WITHOUT opening a
// surrounding transaction. Chunking the per-project trim into its own tx
// (handled by DeleteAuditEventsBefore) bounds the maximum amount of WAL,
// row locks, and advisory locks held by any single retention sweep to one
// tenant's worth of data — a fleet-wide reaper run that wrapped every
// project in one tx would block unrelated tenants for the duration of the
// sweep.
func TestDeleteAuditEventsBeforeExcluding_EnumeratesInAutocommit(t *testing.T) {
	t.Parallel()

	fake := &fakeRetentionDB{}
	q := New(fake)

	_, err := q.DeleteAuditEventsBeforeExcluding(context.Background(), time.Now().UTC(), nil)
	require.Error(t,
		err)
	require.True(t,
		fake.queryCalled,
	)
	assert.False(t,
		fake.beginCalled,
	)

}
