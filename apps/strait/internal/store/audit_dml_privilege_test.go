package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestAuditEventsDMLRestricted_ChecksTableAndColumnPrivileges(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                  string
		hasUpdate             bool
		hasDelete             bool
		hasTruncate           bool
		hasUnsafeColumnUpdate bool
		want                  bool
	}{
		{name: "restricted", want: true},
		{name: "update allowed", hasUpdate: true},
		{name: "delete allowed", hasDelete: true},
		{name: "truncate allowed", hasTruncate: true},
		{name: "non signature column update allowed", hasUnsafeColumnUpdate: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := New(&mockDBTX{
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					for _, privilege := range []string{"UPDATE", "DELETE", "TRUNCATE"} {
						require.Contains(t,
							sql, privilege,
						)
					}
					for _, required := range []string{"has_column_privilege", "attname != 'signature'"} {
						require.Contains(t,
							sql, required,
						)
					}
					return &mockRow{
						scanFn: func(dest ...any) error {
							*dest[0].(*bool) = tc.hasUpdate
							*dest[1].(*bool) = tc.hasDelete
							*dest[2].(*bool) = tc.hasTruncate
							*dest[3].(*bool) = tc.hasUnsafeColumnUpdate
							return nil
						},
					}
				},
			})

			got, err := q.AuditEventsDMLRestricted(context.Background())
			require.NoError(t, err)
			require.Equal(t,
				tc.want,
				got)
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
	require.Error(t,
		err)
	require.Contains(t,
		err.
			Error(), "audit dml privilege check")
}
