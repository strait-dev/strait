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

type bytesRow struct {
	value []byte
}

func (r *bytesRow) Scan(dest ...any) error {
	target, ok := dest[0].(*[]byte)
	if !ok {
		return errors.New("fake: expected []byte scan target")
	}
	*target = append([]byte(nil), r.value...)
	return nil
}

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
	assert.Contains(t,
		err.
			Error(), "refusing to sign under global key")
	assert.True(t,
		fake.insertCalled,
	)
}

type fakeDBTXForSigningKeyCache struct {
	insertCalled  bool
	queryRowCalls int
	ciphertext    []byte
}

func (f *fakeDBTXForSigningKeyCache) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "INSERT INTO audit_signing_keys") {
		f.insertCalled = true
		ciphertext, ok := args[2].([]byte)
		if !ok {
			return pgconn.CommandTag{}, errors.New("fake: expected ciphertext arg")
		}
		f.ciphertext = append([]byte(nil), ciphertext...)
		return pgconn.CommandTag{}, nil
	}
	return pgconn.CommandTag{}, errors.New("fake: unexpected exec")
}

func (f *fakeDBTXForSigningKeyCache) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("fake: query not supported")
}

func (f *fakeDBTXForSigningKeyCache) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "FROM audit_signing_keys") {
		f.queryRowCalls++
		if f.ciphertext == nil {
			return &errRow{err: pgx.ErrNoRows}
		}
		return &bytesRow{value: f.ciphertext}
	}
	return &errRow{err: errors.New("fake: unexpected QueryRow")}
}

func TestResolveSigningKeyForEpoch_CachesResolvedKey(t *testing.T) {
	t.Parallel()

	fake := &fakeDBTXForSigningKeyCache{}
	q := New(fake)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	globalKey, err := DeriveAuditSigningKey("global-sig-secret")
	require.NoError(t, err)
	q.SetAuditSigningKey(globalKey)

	key, err := q.resolveSigningKeyForEpoch(context.Background(), "proj-cache", 0)
	require.NoError(t, err)
	require.Len(t, key, 32)
	require.True(t, fake.insertCalled)
	require.Equal(t, 2, fake.queryRowCalls)

	key[0] ^= 0xff
	cached, err := q.resolveSigningKeyForEpoch(context.Background(), "proj-cache", 0)
	require.NoError(t, err)
	require.Equal(t, 2, fake.queryRowCalls)
	require.NotEqual(t, key[0], cached[0])

	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	reread, err := q.resolveSigningKeyForEpoch(context.Background(), "proj-cache", 0)
	require.NoError(t, err)
	require.Equal(t, 3, fake.queryRowCalls)
	require.Equal(t, cached, reread)
}
