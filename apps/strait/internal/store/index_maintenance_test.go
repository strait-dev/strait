package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestIsMissingIndexError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "undefined table",
			err:  &pgconn.PgError{Code: "42P01"},
			want: true,
		},
		{
			name: "undefined object",
			err:  &pgconn.PgError{Code: "42704"},
			want: true,
		},
		{
			name: "other pg error",
			err:  &pgconn.PgError{Code: "23505"},
			want: false,
		},
		{
			name: "non pg error",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isMissingIndexError(tt.err))
		})
	}
}

func TestIsMissingIndexErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		want bool
	}{
		{name: "undefined table", code: "42P01", want: true},
		{name: "undefined object", code: "42704", want: true},
		{name: "unique violation", code: "23505", want: false},
		{name: "empty code", code: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isMissingIndexErrorCode(tt.code))
		})
	}
}
