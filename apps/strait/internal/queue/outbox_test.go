package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteOutboxInTx_EmptyEntries(t *testing.T) {
	assert.NoError(
		t, WriteOutboxInTx(context.
			Background(), nil, nil))
	assert.NoError(
		t, WriteOutboxInTx(context.
			Background(), nil, []OutboxEntry{}))
}

func TestWriteOutboxInTx_MissingProjectIDRejected(t *testing.T) {
	err := WriteOutboxInTx(context.Background(), nil, []OutboxEntry{{
		JobID: "job1",
	}})
	assert.Error(t,
		err)
}

func TestWriteOutboxInTx_MissingJobIDRejected(t *testing.T) {
	err := WriteOutboxInTx(context.Background(), nil, []OutboxEntry{{
		ProjectID: "p1",
	}})
	assert.Error(t,
		err)
}

func TestUniqueJobIDs_DeduplicatesInFirstSeenOrder(t *testing.T) {
	got := uniqueJobIDs([]OutboxEntry{
		{JobID: "job-1"},
		{JobID: "job-2"},
		{JobID: "job-1"},
		{JobID: "job-3"},
	})
	want := []string{"job-1", "job-2", "job-3"}
	require.Len(t,
		got, len(want))

	for i := range want {
		require.Equal(t,
			want[i],
			got[i])
	}
}

func TestWriteOutboxInTx_UsesBatchForInserts(t *testing.T) {
	results := &outboxBatchResults{}
	tx := &outboxMockTx{
		rows:         &outboxRows{ids: []string{"job-1", "job-2"}},
		batchResults: results,
	}
	entries := []OutboxEntry{
		{ID: "outbox-1", ProjectID: "project-1", JobID: "job-1", Metadata: map[string]any{"source": "test"}},
		{ID: "outbox-2", ProjectID: "project-1", JobID: "job-1"},
		{ID: "outbox-3", ProjectID: "project-1", JobID: "job-2"},
	}
	require.NoError(t, WriteOutboxInTx(context.
		Background(), tx, entries))
	require.Equal(t, 0, tx.execCalls)
	require.NotNil(
		t, tx.sentBatch,
	)
	require.Equal(t,
		len(entries), tx.sentBatch.
			Len())
	require.Equal(t,
		len(entries), results.
			execCalls,
	)
	require.True(t,
		results.closeCalled,
	)

	first := tx.sentBatch.QueuedQueries[0]
	require.Contains(t,
		first.
			SQL, "ON CONFLICT (id) DO NOTHING")
	require.Equal(t,
		"outbox-1",
		first.Arguments[0],
	)
}

func TestWriteOutboxInTx_BatchExecErrorIncludesEntryID(t *testing.T) {
	writeErr := errors.New("insert failed")
	results := &outboxBatchResults{execErrAt: 2, err: writeErr}
	tx := &outboxMockTx{
		rows:         &outboxRows{ids: []string{"job-1"}},
		batchResults: results,
	}
	entries := []OutboxEntry{
		{ID: "outbox-1", ProjectID: "project-1", JobID: "job-1"},
		{ID: "outbox-2", ProjectID: "project-1", JobID: "job-1"},
	}

	err := WriteOutboxInTx(context.Background(), tx, entries)
	require.ErrorIs(t,
		err, writeErr)
	require.Contains(t,
		err.
			Error(), "outbox-2")
	require.True(t,
		results.closeCalled,
	)
}

type outboxMockTx struct {
	rows         pgx.Rows
	batchResults pgx.BatchResults
	sentBatch    *pgx.Batch
	execCalls    int
}

func (m *outboxMockTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("nested") }
func (m *outboxMockTx) Commit(context.Context) error          { return nil }
func (m *outboxMockTx) Rollback(context.Context) error        { return nil }
func (m *outboxMockTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *outboxMockTx) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	m.sentBatch = b
	return m.batchResults
}
func (m *outboxMockTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }
func (m *outboxMockTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *outboxMockTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	m.execCalls++
	return pgconn.CommandTag{}, nil
}
func (m *outboxMockTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return m.rows, nil
}
func (m *outboxMockTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (m *outboxMockTx) Conn() *pgx.Conn                                  { return nil }

type outboxRows struct {
	ids    []string
	index  int
	closed bool
}

func (r *outboxRows) Close()                                       { r.closed = true }
func (r *outboxRows) Err() error                                   { return nil }
func (r *outboxRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *outboxRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *outboxRows) Next() bool {
	if r.index >= len(r.ids) {
		r.Close()
		return false
	}
	r.index++
	return true
}
func (r *outboxRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("outboxRows: expected one destination")
	}
	id, ok := dest[0].(*string)
	if !ok {
		return errors.New("outboxRows: destination is not *string")
	}
	*id = r.ids[r.index-1]
	return nil
}
func (r *outboxRows) Values() ([]any, error) { return []any{r.ids[r.index-1]}, nil }
func (r *outboxRows) RawValues() [][]byte    { return nil }
func (r *outboxRows) Conn() *pgx.Conn        { return nil }

type outboxBatchResults struct {
	execCalls   int
	execErrAt   int
	err         error
	closeCalled bool
}

func (r *outboxBatchResults) Exec() (pgconn.CommandTag, error) {
	r.execCalls++
	if r.execErrAt > 0 && r.execCalls == r.execErrAt {
		return pgconn.CommandTag{}, r.err
	}
	return pgconn.CommandTag{}, nil
}
func (r *outboxBatchResults) Query() (pgx.Rows, error) { return nil, nil }
func (r *outboxBatchResults) QueryRow() pgx.Row        { return nil }
func (r *outboxBatchResults) Close() error {
	r.closeCalled = true
	return nil
}
