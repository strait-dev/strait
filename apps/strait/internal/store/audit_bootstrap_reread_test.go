package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDBTXForBootstrap implements DBTX and simulates the pathological
// case: INSERT ON CONFLICT DO NOTHING reports success, but the
// subsequent SELECT returns pgx.ErrNoRows as if the row is not there.
// The guarded path must surface an error rather than silently falling
// back to the global key and producing a chain the verifier cannot
// reproduce.
type fakeDBTXForBootstrap struct {
	insertCalled bool
	// After an INSERT, the next QueryRow from GetAuditSigningKey should
	// behave as if the row vanished; we return ErrNoRows when this
	// flag is set.
	rereadReturnsNoRows bool
}

func (f *fakeDBTXForBootstrap) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "INSERT INTO audit_signing_keys") {
		f.insertCalled = true
		f.rereadReturnsNoRows = true
		return pgconn.CommandTag{}, nil
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTXForBootstrap) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("fake: query not supported")
}

func (f *fakeDBTXForBootstrap) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	// Two QueryRow calls exercise this path. First the pre-insert
	// lookup by resolveSigningKeyForEpoch's initial GetAuditSigningKey
	// (returns nil because no row exists yet). Second, after the
	// INSERT bump's ON CONFLICT DO NOTHING, the re-read which we force
	// to ErrNoRows.
	if strings.Contains(sql, "FROM audit_signing_keys") {
		if f.rereadReturnsNoRows {
			return &errRow{err: pgx.ErrNoRows}
		}
		return &errRow{err: pgx.ErrNoRows}
	}
	return &errRow{err: errors.New("fake: unexpected QueryRow")}
}

type errRow struct{ err error }

func (r *errRow) Scan(_ ...any) error { return r.err }

// TestResolveSigningKeyForEpoch_InsertedButNotReadBack_Errors asserts the
// bootstrap path surfaces an explicit error when the post-INSERT re-read
// returns no row. Silently falling back to the global q.auditSigningKey
// would make the signer and verifier disagree on the key for every
// subsequent event signed under this path — they would all fail
// signature comparison on VerifyAuditChain.
func TestResolveSigningKeyForEpoch_InsertedButNotReadBack_Errors(t *testing.T) {
	t.Parallel()

	fake := &fakeDBTXForBootstrap{}
	q := New(fake)
	q.secretEncryptionKey = "test-encryption-key-32bytes!!!!"
	globalKey, _ := DeriveAuditSigningKey("global-sig-secret")
	q.auditSigningKey = globalKey

	_, err := q.resolveSigningKeyForEpoch(context.Background(), "proj-ghost", 0)
	require.Error(t,
		err)
	assert.True(t,
		strings.Contains(err.
			Error(), "refusing to sign under global key",
		))
	assert.True(t,
		fake.insertCalled,
	)

}
