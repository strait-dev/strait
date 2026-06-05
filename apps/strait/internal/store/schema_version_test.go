package store

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

type fakeSchemaDB struct {
	version int
	noRows  bool
	err     error
}

func (f *fakeSchemaDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeSchemaDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not used")
}
func (f *fakeSchemaDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if f.err != nil {
		return &fakeSchemaRow{err: f.err}
	}
	if f.noRows {
		return &fakeSchemaRow{err: pgx.ErrNoRows}
	}
	return &fakeSchemaRow{version: f.version}
}

type fakeSchemaRow struct {
	version int
	err     error
}

func (r *fakeSchemaRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if v, ok := dest[0].(*int); ok {
		*v = r.version
	}
	return nil
}

func TestCheckSchemaVersion_Match(t *testing.T) {
	q := New(&fakeSchemaDB{version: domain.ExpectedSchemaVersion})
	assert.NoError(
		t, q.CheckSchemaVersion(context.Background(), domain.
			ExpectedSchemaVersion))
}

func TestCheckSchemaVersion_BinaryBehind(t *testing.T) {
	q := New(&fakeSchemaDB{version: domain.ExpectedSchemaVersion + 1})
	err := q.CheckSchemaVersion(context.Background(), domain.ExpectedSchemaVersion)
	assert.ErrorIs(t,
		err, ErrSchemaMismatch)
}

func TestCheckSchemaVersion_BinaryAhead(t *testing.T) {
	q := New(&fakeSchemaDB{version: domain.ExpectedSchemaVersion - 6})
	err := q.CheckSchemaVersion(context.Background(), domain.ExpectedSchemaVersion)
	assert.ErrorIs(t,
		err, ErrSchemaMismatch)
}

func TestCheckSchemaVersion_NoTable(t *testing.T) {
	q := New(&fakeSchemaDB{noRows: true})
	assert.NoError(
		t, q.CheckSchemaVersion(context.Background(), domain.
			ExpectedSchemaVersion))
}

func TestCheckSchemaVersion_SkipOnZero(t *testing.T) {
	q := New(&fakeSchemaDB{version: 999})
	assert.NoError(
		t, q.CheckSchemaVersion(context.Background(), 0),
	)
}
