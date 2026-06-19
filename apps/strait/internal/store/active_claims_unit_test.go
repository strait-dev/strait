package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestActiveClaimCleanupHelpersUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limit     int
		wantLimit int
		wantSQL   string
		call      func(context.Context, *Queries, int) (int64, error)
	}{
		{
			name:      "delete inactive active claims default limit",
			limit:     0,
			wantLimit: 10000,
			wantSQL:   "DELETE FROM job_run_active_claims",
			call: func(ctx context.Context, q *Queries, limit int) (int64, error) {
				return q.DeleteInactiveActiveClaims(ctx, limit)
			},
		},
		{
			name:      "delete inactive ready events explicit limit",
			limit:     25,
			wantLimit: 25,
			wantSQL:   "DELETE FROM job_run_ready_events",
			call: func(ctx context.Context, q *Queries, limit int) (int64, error) {
				return q.DeleteInactiveReadyEvents(ctx, limit)
			},
		},
		{
			name:      "compact superseded priority events default negative limit",
			limit:     -1,
			wantLimit: 10000,
			wantSQL:   "FROM job_run_priority_events newer",
			call: func(ctx context.Context, q *Queries, limit int) (int64, error) {
				return q.CompactSupersededPriorityEvents(ctx, limit)
			},
		},
		{
			name:      "compact superseded visibility events explicit limit",
			limit:     11,
			wantLimit: 11,
			wantSQL:   "FROM job_run_visibility_events newer",
			call: func(ctx context.Context, q *Queries, limit int) (int64, error) {
				return q.CompactSupersededVisibilityEvents(ctx, limit)
			},
		},
		{
			name:      "compact superseded run cache versions explicit limit",
			limit:     7,
			wantLimit: 7,
			wantSQL:   "DELETE FROM job_run_cache_versions",
			call: func(ctx context.Context, q *Queries, limit int) (int64, error) {
				return q.CompactSupersededRunCacheVersions(ctx, limit)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, tt.wantSQL)
				require.Len(t, args, 1)
				require.Equal(t, tt.wantLimit, args[0])
				return pgconn.NewCommandTag("DELETE 3"), nil
			}}

			got, err := tt.call(context.Background(), New(db), tt.limit)
			require.NoError(t, err)
			require.EqualValues(t, 3, got)
		})
	}
}

func TestActiveClaimCleanupHelpersWrapExecErrorsUnit(t *testing.T) {
	t.Parallel()

	execErr := errors.New("exec failed")
	tests := []struct {
		name string
		want string
		call func(context.Context, *Queries) (int64, error)
	}{
		{
			name: "delete inactive active claims",
			want: "delete inactive active claims",
			call: func(ctx context.Context, q *Queries) (int64, error) {
				return q.DeleteInactiveActiveClaims(ctx, 1)
			},
		},
		{
			name: "delete inactive ready events",
			want: "delete inactive ready events",
			call: func(ctx context.Context, q *Queries) (int64, error) {
				return q.DeleteInactiveReadyEvents(ctx, 1)
			},
		},
		{
			name: "compact superseded priority events",
			want: "compact superseded priority events",
			call: func(ctx context.Context, q *Queries) (int64, error) {
				return q.CompactSupersededPriorityEvents(ctx, 1)
			},
		},
		{
			name: "compact superseded visibility events",
			want: "compact superseded visibility events",
			call: func(ctx context.Context, q *Queries) (int64, error) {
				return q.CompactSupersededVisibilityEvents(ctx, 1)
			},
		},
		{
			name: "compact superseded run cache versions",
			want: "compact superseded run cache versions",
			call: func(ctx context.Context, q *Queries) (int64, error) {
				return q.CompactSupersededRunCacheVersions(ctx, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			}}

			got, err := tt.call(context.Background(), New(db))
			require.ErrorIs(t, err, execErr)
			require.Contains(t, err.Error(), tt.want)
			require.Zero(t, got)
		})
	}
}
