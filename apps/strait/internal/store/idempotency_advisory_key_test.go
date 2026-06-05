package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyAdvisoryKey_Stable(t *testing.T) {
	t.Parallel()
	a := idempotencyAdvisoryKey("proj-1", "key-abc")
	b := idempotencyAdvisoryKey("proj-1", "key-abc")
	require.Equal(
		t, b, a)

}

func TestIdempotencyAdvisoryKey_DifferentPairsDiffer(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"proj-1", "key-a"},
		{"proj-1", "key-b"},
		{"proj-2", "key-a"},
		{"", ""},
		{"proj-1", ""},
		{"", "key-a"},
	}
	seen := make(map[int64][2]string, len(pairs))
	for _, p := range pairs {
		k := idempotencyAdvisoryKey(p[0], p[1])
		if prev, ok := seen[k]; ok {
			require.Failf(t, "test failure",

				"advisory key collision: %v vs %v -> %d", prev, p, k)
		}
		seen[k] = p
	}
}

func TestIdempotencyAdvisoryKey_NoSeparatorInjection(t *testing.T) {
	t.Parallel()
	a := idempotencyAdvisoryKey("a", "b:c")
	b := idempotencyAdvisoryKey("a:b", "c")
	require.NotEqual(t, b, a)

}

func TestIsIdempotencyTransientError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("plain"), false},
		{"40001 serialization", &pgconn.PgError{Code: "40001"}, true},
		{"40P01 deadlock", &pgconn.PgError{Code: "40P01"}, true},
		{"55P03 lock_timeout", &pgconn.PgError{Code: "55P03"}, true},
		{"23505 unique violation", &pgconn.PgError{Code: "23505"}, false},
		{"42701 duplicate column", &pgconn.PgError{Code: "42701"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t,
				tt.want,
				isIdempotencyTransientError(tt.err),
			)

		})
	}
}

func TestIsIdempotencyTransientError_WrappedPgError(t *testing.T) {
	t.Parallel()
	wrapped := errors.Join(errors.New("outer"), &pgconn.PgError{Code: "40001"})
	require.True(t,
		isIdempotencyTransientError(wrapped))

}
