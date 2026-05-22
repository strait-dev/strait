package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIdempotencyAdvisoryKey_Stable(t *testing.T) {
	t.Parallel()
	a := idempotencyAdvisoryKey("proj-1", "key-abc")
	b := idempotencyAdvisoryKey("proj-1", "key-abc")
	if a != b {
		t.Fatalf("advisory key not stable: %d vs %d", a, b)
	}
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
			t.Fatalf("advisory key collision: %v vs %v -> %d", prev, p, k)
		}
		seen[k] = p
	}
}

func TestIdempotencyAdvisoryKey_NoSeparatorInjection(t *testing.T) {
	t.Parallel()
	a := idempotencyAdvisoryKey("a", "b:c")
	b := idempotencyAdvisoryKey("a:b", "c")
	if a == b {
		t.Fatalf("length-prefix should prevent separator-injection collision, got %d", a)
	}
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
			if got := isIdempotencyTransientError(tt.err); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsIdempotencyTransientError_WrappedPgError(t *testing.T) {
	t.Parallel()
	wrapped := errors.Join(errors.New("outer"), &pgconn.PgError{Code: "40001"})
	if !isIdempotencyTransientError(wrapped) {
		t.Fatal("errors.As should find wrapped pg error")
	}
}
