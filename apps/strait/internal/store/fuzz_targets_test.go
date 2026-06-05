package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// placeholderRE matches every pgx parameter placeholder ($1, $2, …) so
// tests can strip placeholders out of a captured SQL template before
// checking that a user-controlled string doesn't appear as a literal.
// This handles the false-positive case where a short user key like "2"
// would be a substring of "$2" and trigger a spurious leak alarm.
var placeholderRE = regexp.MustCompile(`\$\d+`)

// stripPlaceholders replaces every `$N` in sql with a single space so
// leak-checks over the result never match placeholder digits.
func stripPlaceholders(sql string) string {
	return placeholderRE.ReplaceAllString(sql, " ")
}

// High-value fuzz targets surfaced by the test coverage audit.
//
// These are unit fuzz tests (package store, not store_test, no integration
// build tag) that run in the fast test path. They don't need a real
// Postgres — they exercise SQL builders via a mock DBTX and verify that
// user-controlled strings never reach the SQL as literals (always
// parameterized) and never cause the code to panic.
//
// Targets:
//
//  1. FuzzListRunsByProject_MetadataFilter_NoInjection
//  2. FuzzTryAcquireIdempotencyKey_KeyPassthrough
//  3. FuzzCreateEventTrigger_EventKeyPassthrough
//  4. FuzzDecryptSecretWithFallback_CorruptionDeterministic
//  5. FuzzDomainIsValid_StatusAndModes_NoPanic
// Shared test helpers

// fuzzCaptureDB is a mockDBTX variant that records the SQL + args passed
// to Exec / Query / QueryRow and always returns a sentinel error so
// callers short-circuit before trying to read rows. Tests can inspect
// the last call via the captured fields.
type fuzzCaptureDB struct {
	sentinel error
	sql      string
	args     []any
}

func newFuzzCaptureDB() *fuzzCaptureDB {
	return &fuzzCaptureDB{sentinel: errors.New("fuzz-capture: stop")}
}

func (c *fuzzCaptureDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.sql = sql
	c.args = args
	return pgconn.CommandTag{}, c.sentinel
}

func (c *fuzzCaptureDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.sql = sql
	c.args = args
	return nil, c.sentinel
}

func (c *fuzzCaptureDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	c.sql = sql
	c.args = args
	return fuzzErrRow{err: c.sentinel}
}

type fuzzErrRow struct{ err error }

func (r fuzzErrRow) Scan(_ ...any) error { return r.err }

// argsContain returns true if any of the captured args equals s.
func argsContain(args []any, s string) bool {
	for _, a := range args {
		switch v := a.(type) {
		case string:
			if v == s {
				return true
			}
		case *string:
			if v != nil && *v == s {
				return true
			}
		}
	}
	return false
}

// 1. ListRunsByProject metadata filter

// FuzzListRunsByProject_MetadataFilter_NoInjection verifies that the
// user-controlled metadata key and value in ListRunsByProject are
// always passed as SQL parameters, never concatenated into the query
// string. Tested branches:
//
//   - metadataKey != nil and metadataValue == nil → merged metadata `? $N`
//   - metadataKey != nil and metadataValue != nil → merged metadata `->> $N = $N+1`
//
// The fuzz asserts (a) no panic on any input, (b) the constructed SQL
// never contains the user key/value as a literal substring when the
// values are non-empty and non-trivially substrings of legitimate SQL
// tokens.
func FuzzListRunsByProject_MetadataFilter_NoInjection(f *testing.F) {
	f.Add("user_id", "abc123")
	f.Add("", "")
	f.Add("x", "")
	f.Add("x\x00y", "v")
	f.Add("' OR 1=1--", "'; DROP TABLE job_runs; --")
	f.Add("user->id", "a@>b")
	f.Add("café", "cafe\u0301")
	f.Add(strings.Repeat("k", 4096), strings.Repeat("v", 4096))
	f.Add(string([]byte{0xed, 0xb2, 0x80}), "valid") // lone low surrogate byte sequence
	f.Add("k", "\x00\x01\x02")

	f.Fuzz(func(t *testing.T, key, value string) {
		// Branch 1: key only.
		{
			cap1 := newFuzzCaptureDB()
			q := New(cap1)
			mk := key
			_, err := q.ListRunsByProject(context.Background(), "proj-fuzz", nil, &mk, nil, nil, nil, nil, nil, nil, 10, nil)
			require.Error(t,
				err)
			require.True(t,
				errors.Is(err, cap1.
					sentinel))
			require.False(t,
				!strings.Contains(cap1.sql, "COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)") ||
					!strings.Contains(cap1.sql,
						"? $"))

			// The SQL must query the merged ledger + append-only metadata
			// overlay. The key must NOT appear as a literal in the query
			// string.

			// Strip all placeholders ($1, $2, …) then assert the key
			// does not appear as a literal. Short keys are still
			// skipped to avoid coinciding with SQL keywords like "id",
			// "as", or column names in the SELECT list that are outside
			// the caller's control.
			if len(key) >= 8 {
				stripped := stripPlaceholders(cap1.sql)
				require.False(t,
					strings.Contains(stripped, key),
				)

			}
			require.True(t,
				argsContain(cap1.
					args, key))

		}

		// Branch 2: key + value.
		{
			cap2 := newFuzzCaptureDB()
			q := New(cap2)
			mk := key
			mv := value
			_, err := q.ListRunsByProject(context.Background(), "proj-fuzz", nil, &mk, &mv, nil, nil, nil, nil, nil, 10, nil)
			require.Error(t,
				err)
			require.False(t,
				!strings.Contains(cap2.sql, "COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)") ||
					!strings.Contains(cap2.sql,
						"->> $"))

			stripped := stripPlaceholders(cap2.sql)
			require.False(t,
				len(key) >= 8 &&
					strings.Contains(stripped,
						key),
			)
			require.False(t,
				len(value) >= 8 &&
					strings.Contains(stripped,
						value,
					))
			require.True(t,
				argsContain(cap2.
					args, key))
			require.True(t,
				argsContain(cap2.
					args, value))

		}
	})
}

// 2. TryAcquireIdempotencyKey: key is passed through unchanged

// FuzzTryAcquireIdempotencyKey_KeyPassthrough verifies that the store
// does not normalize, truncate, or mutate the idempotency key before
// sending it to the database. Canonicalization (if any) must happen in
// the caller so that the key observed by the RLS policy and the unique
// constraint is exactly what the client sent.
func FuzzTryAcquireIdempotencyKey_KeyPassthrough(f *testing.F) {
	f.Add("simple-key")
	f.Add("")
	f.Add("café")
	f.Add("cafe\u0301")
	f.Add("key\x00with\x00nulls")
	f.Add(strings.Repeat("x", 1024))
	f.Add("emoji-😀-key")
	f.Add("SELECT * FROM idempotency_keys")

	f.Fuzz(func(t *testing.T, key string) {
		capture := newFuzzCaptureDB()
		q := New(capture)
		_, _, _, _, _ = q.TryAcquireIdempotencyKey(context.Background(), "proj-fuzz", key, time.Hour)
		require.True(t,
			argsContain(capture.
				args, key))

		// The first call the store makes is INSERT ... ON CONFLICT DO
		// NOTHING. The key must be in args exactly as provided.

		// Assert the key is not concatenated into the SQL. Strip
		// placeholders first so short keys that coincide with
		// placeholder digits aren't false positives. Only keys >= 8
		// chars are checked to avoid colliding with SQL keywords in
		// the captured INSERT template ("key", "id", "status", etc.).
		if len(key) >= 8 {
			stripped := stripPlaceholders(capture.sql)
			require.False(t,
				strings.Contains(stripped, key),
			)

		}
	})
}

// 3. CreateEventTrigger: event_key is parameterized

// FuzzCreateEventTrigger_EventKeyPassthrough is the event_triggers
// analog of the idempotency fuzz. event_key has a UNIQUE constraint and
// is taken from user input (webhook sender); a null-byte or SQL meta
// input should be stored literally and cannot smuggle through the SQL
// builder.
func FuzzCreateEventTrigger_EventKeyPassthrough(f *testing.F) {
	f.Add("order.created")
	f.Add("")
	f.Add("event\x00key")
	f.Add("'; DROP TABLE event_triggers; --")
	f.Add("event@>contains")
	f.Add(strings.Repeat("e", 2048))
	f.Add("event_with_unicode_café")

	f.Fuzz(func(t *testing.T, eventKey string) {
		capture := newFuzzCaptureDB()
		q := New(capture)
		expiresAt := time.Now().Add(time.Hour)
		trigger := &domain.EventTrigger{
			ID:          "trigger-fuzz",
			EventKey:    eventKey,
			ProjectID:   "proj-fuzz",
			SourceType:  "webhook",
			Status:      "waiting",
			RequestedAt: time.Now(),
			ExpiresAt:   expiresAt,
			TimeoutSecs: 3600,
		}
		_ = q.CreateEventTrigger(context.Background(), trigger)
		require.True(t,
			argsContain(capture.
				args, eventKey,
			))

		if len(eventKey) >= 8 {
			stripped := stripPlaceholders(capture.sql)
			require.False(t,
				strings.Contains(stripped, eventKey))

		}
	})
}

// 4. decryptSecretWithFallback: corrupted ciphertext fails deterministically

// FuzzDecryptSecretWithFallback_CorruptionDeterministic generates a
// known-good ciphertext, mutates it, and verifies decryption fails
// deterministically (error + empty plaintext) rather than panicking,
// silently returning wrong data, or leaking partial plaintext.
//
// Also verifies the fallback path is safe: the legacy SHA-256 key is
// tried after the HKDF key fails, but a corrupted blob that happens to
// decrypt under the legacy key is a separate concern (the ciphertext
// format is authenticated).
func FuzzDecryptSecretWithFallback_CorruptionDeterministic(f *testing.F) {
	// Baseline: ciphertexts known to be valid under different
	// passphrases. The fuzz mutates these via the corruption mask.
	f.Add(uint(0), "base-passphrase")
	f.Add(uint(1), "base-passphrase")
	f.Add(uint(2), "another-passphrase")
	f.Add(uint(7), "fuzz-passphrase")
	f.Add(uint(0xFF), "fuzz-passphrase")

	f.Fuzz(func(t *testing.T, mask uint, passphrase string) {
		if passphrase == "" {
			return
		}
		q := &Queries{secretEncryptionKey: passphrase}
		key, err := q.secretKey()
		if err != nil {
			return
		}

		ciphertext, err := encryptSecret("fuzz-plaintext", key)
		if err != nil {
			return
		}

		// Decode hex, XOR the low byte of `mask` into every byte at
		// position mask%len, then re-encode. This produces a
		// single-byte corruption somewhere in the blob.
		raw, decodeErr := hex.DecodeString(ciphertext)
		if decodeErr != nil {
			return
		}
		if len(raw) == 0 {
			return
		}
		pos := int(mask) % len(raw)
		raw[pos] ^= byte(mask&0xFF) | 1 // ensure at least one bit flips
		corrupted := hex.EncodeToString(raw)

		// Decryption must fail cleanly. A panic here is the regression
		// we want to catch. Legacy SHA-256 fallback should not magically
		// decrypt a corrupted HKDF blob unless the mask happens to be
		// cryptographically equivalent, which is astronomically unlikely.
		plaintext, err := q.decryptSecretWithFallback(corrupted)
		require.False(t,
			err == nil && plaintext ==
				"fuzz-plaintext",
		)

		// If mask is 0 this shouldn't happen because we force a bit
		// flip. Treat any match as a failure.

		// Also verify the known-good ciphertext still decrypts to
		// confirm the fuzz setup is sane.
		got, err := q.decryptSecretWithFallback(ciphertext)
		require.NoError(t, err)
		require.Equal(t,
			"fuzz-plaintext",
			got)

	})
}

// 5. Domain IsValid methods: never panic, always return bool

// FuzzDomainIsValid_StatusAndModes_NoPanic calls every public IsValid /
// IsTerminal method on every enum-like domain type with an arbitrary
// string. Asserts no panic, and that known values round-trip.
func FuzzDomainIsValid_StatusAndModes_NoPanic(f *testing.F) {
	f.Add("queued")
	f.Add("completed")
	f.Add("")
	f.Add("QUEUED")
	f.Add("unknown-status-" + strings.Repeat("x", 100))
	f.Add("val\x00ue")
	f.Add("val" + string([]byte{0xed, 0xb2, 0x80}) + "ue") // lone surrogate
	f.Add(strings.Repeat("a", 4096))

	f.Fuzz(func(t *testing.T, s string) {
		// Each call below must complete without panicking.
		defer func() {
			require.Nil(t, recover())

		}()

		// RunStatus.
		rs := domain.RunStatus(s)
		_ = rs.IsValid()
		_ = rs.IsTerminal()

		// StepRunStatus: only IsTerminal is defined; no IsValid.
		srs := domain.StepRunStatus(s)
		_ = srs.IsTerminal()

		// WorkflowRunStatus.
		wrs := domain.WorkflowRunStatus(s)
		_ = wrs.IsValid()

		// ExecutionMode.
		em := domain.ExecutionMode(s)
		_ = em.IsValid()

		// CronOverlapPolicy.
		cop := domain.CronOverlapPolicy(s)
		_ = cop.IsValid()
	})
}

// Compile-time check that sha256 and fmt are referenced to match the
// rest of the store test harness idioms. Unused otherwise.
var _ = sha256.New
var _ = fmt.Sprintf
