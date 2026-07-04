package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queryRecorderDBTX records all SQL queries that pass through it. Used to
// assert that ORDER BY clauses include the expected tiebreaker columns.
type queryRecorderDBTX struct {
	onQuery    func(sql string)
	onQueryRow func(sql string)
}

func (r *queryRecorderDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (r *queryRecorderDBTX) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if r.onQuery != nil {
		r.onQuery(sql)
	}
	return &emptyRows{}, nil
}

func (r *queryRecorderDBTX) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if r.onQueryRow != nil {
		r.onQueryRow(sql)
	}
	// Grant the advisory locks CreateAuditEvent takes before its tail-read so
	// execution flows past lock acquisition to the query under test. Other
	// reads scan as no-ops: these tests assert on the SQL text, not row data.
	if strings.Contains(sql, "pg_try_advisory_xact_lock") {
		return advisoryGrantedRow{}
	}
	return noopScanRow{}
}

type emptyRows struct{}

func (e *emptyRows) Close()                                       {}
func (e *emptyRows) Err() error                                   { return nil }
func (e *emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (e *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (e *emptyRows) Values() ([]any, error)                       { return nil, nil }
func (e *emptyRows) RawValues() [][]byte                          { return nil }
func (e *emptyRows) Conn() *pgx.Conn                              { return nil }
func (e *emptyRows) Next() bool                                   { return false }
func (e *emptyRows) Scan(_ ...any) error                          { return nil }

// advisoryGrantedRow satisfies the pg_try_advisory_xact_lock read by reporting
// the lock as acquired, so AcquireAdvisoryLock returns instead of spinning.
type advisoryGrantedRow struct{}

func (advisoryGrantedRow) Scan(dest ...any) error {
	if len(dest) == 1 {
		if acquired, ok := dest[0].(*bool); ok {
			*acquired = true
		}
	}
	return nil
}

// noopScanRow leaves scan destinations untouched and reports success.
type noopScanRow struct{}

func (noopScanRow) Scan(_ ...any) error { return nil }

func domainAuditEventForTest() domain.AuditEvent {
	return domain.AuditEvent{
		ProjectID:    "proj-test",
		ActorID:      "actor-1",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{}`),
	}
}

// TestVerifyAuditChain_QueryHasIDTiebreaker asserts that the VerifyAuditChain
// SQL query orders by shard_id, rotation_epoch, created_at, and id. Without
// shard_id first the verifier interleaves rows from independent sub-chains and
// produces a false-positive chain break. Without "id ASC" as the final tiebreaker
// two events sharing the same (shard_id, rotation_epoch, created_at) can be
// scanned in non-deterministic order.
func TestVerifyAuditChain_QueryHasIDTiebreaker(t *testing.T) {
	t.Parallel()

	expected := "ORDER BY shard_id ASC, rotation_epoch ASC, created_at ASC, id ASC"
	var lastQuery string
	fake := &queryRecorderDBTX{
		onQuery: func(sql string) { lastQuery = sql },
	}
	q := New(fake)
	key := make([]byte, 32)
	q.SetAuditSigningKey(key)

	_, _ = q.VerifyAuditChain(context.Background(), "proj-test")
	assert.Contains(t,
		lastQuery, expected)
}

// TestVerifyAuditChainIncremental_QueryHasIDTiebreaker is the same guard for
// the incremental verify path. When no checkpoint exists, the incremental
// path falls back to the full verify, so the same ORDER BY applies.
func TestVerifyAuditChainIncremental_QueryHasIDTiebreaker(t *testing.T) {
	t.Parallel()

	expected := "ORDER BY shard_id ASC, rotation_epoch ASC, created_at ASC, id ASC"
	var lastQuery string
	fake := &queryRecorderDBTX{
		onQuery: func(sql string) { lastQuery = sql },
	}
	q := New(fake)
	key := make([]byte, 32)
	q.SetAuditSigningKey(key)

	_, _ = q.VerifyAuditChainIncremental(context.Background(), "proj-test")
	assert.Contains(t,
		lastQuery, expected)
}

// TestCreateAuditEvent_TailReadHasIDTiebreaker verifies the tail-read
// query (SELECT signature ... ORDER BY ... LIMIT 1) includes id DESC so
// ties on (rotation_epoch, created_at) resolve deterministically.
func TestCreateAuditEvent_TailReadHasIDTiebreaker(t *testing.T) {
	t.Parallel()

	expected := "id DESC LIMIT 1"
	var queries []string
	fake := &queryRecorderDBTX{
		onQueryRow: func(sql string) { queries = append(queries, sql) },
	}
	q := New(fake)
	key := make([]byte, 32)
	q.SetAuditSigningKey(key)

	ev := domainAuditEventForTest()
	_ = q.CreateAuditEvent(context.Background(), &ev)

	found := false
	for _, sql := range queries {
		// The SQL is multi-line; collapse whitespace so substring checks are
		// not defeated by the newline between SELECT and its columns.
		normalized := strings.Join(strings.Fields(sql), " ")
		// Identify the chain tail-read by its previous-hash lookup; the INSERT
		// and advisory-lock queries do not read the signature back.
		if strings.Contains(normalized, "SELECT signature FROM audit_events") {
			if strings.Contains(normalized, expected) {
				found = true
			} else {
				assert.Failf(t, "tail-read missing tiebreaker",
					"tail-read query missing %q.\nGot: %s", expected, normalized)
			}
		}
	}
	// Fail loudly rather than skipping. Skipping here would let a refactor that
	// stops routing CreateAuditEvent's tail-read through QueryRow (or drops it
	// entirely) silently lose coverage of the id-tiebreaker determinism the
	// audit hash chain depends on.
	require.NotEmpty(t, queries, "CreateAuditEvent should issue at least one QueryRow (the tail-read)")
	assert.True(t, found, "chain tail-read (SELECT signature FROM audit_events ...) not found among the captured QueryRow calls")
}

// TestListAuditEvents_QueryHasIDTiebreaker verifies that the ListAuditEvents
// ORDER BY clause includes id as a tiebreaker for deterministic pagination.
func TestListAuditEvents_QueryHasIDTiebreaker(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		ascending bool
		expected  string
	}{
		{"descending", false, "ORDER BY created_at DESC, id DESC"},
		{"ascending", true, "ORDER BY created_at ASC, id ASC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var lastQuery string
			fake := &queryRecorderDBTX{
				onQuery: func(sql string) { lastQuery = sql },
			}
			q := New(fake)

			_, _ = q.ListAuditEvents(context.Background(), "proj-test", "", "", "", 50, nil, nil, nil, tc.ascending)
			assert.Contains(t,
				lastQuery, tc.expected,
			)
		})
	}
}
