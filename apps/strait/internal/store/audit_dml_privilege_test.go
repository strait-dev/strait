package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAuditEventsDMLRestricted_ChecksUpdateDeleteAndTruncate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		hasUpdate   bool
		hasDelete   bool
		hasTruncate bool
		want        bool
	}{
		{name: "restricted", want: true},
		{name: "update allowed", hasUpdate: true},
		{name: "delete allowed", hasDelete: true},
		{name: "truncate allowed", hasTruncate: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := New(&mockDBTX{
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					for _, privilege := range []string{"UPDATE", "DELETE", "TRUNCATE"} {
						if !strings.Contains(sql, privilege) {
							t.Fatalf("privilege query %q missing %s check", sql, privilege)
						}
					}
					return &mockRow{
						scanFn: func(dest ...any) error {
							*dest[0].(*bool) = tc.hasUpdate
							*dest[1].(*bool) = tc.hasDelete
							*dest[2].(*bool) = tc.hasTruncate
							return nil
						},
					}
				},
			})

			got, err := q.AuditEventsDMLRestricted(context.Background())
			if err != nil {
				t.Fatalf("AuditEventsDMLRestricted() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("AuditEventsDMLRestricted() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuditEventsDMLRestricted_PropagatesProbeErrors(t *testing.T) {
	t.Parallel()

	q := New(&mockDBTX{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return errors.New("catalog unavailable") }}
		},
	})

	_, err := q.AuditEventsDMLRestricted(context.Background())
	if err == nil {
		t.Fatal("expected probe error")
	}
	if !strings.Contains(err.Error(), "audit dml privilege check") {
		t.Fatalf("error = %v, want audit dml privilege context", err)
	}
}
