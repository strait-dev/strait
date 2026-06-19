package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestGetEventTriggerStats(t *testing.T) {
	t.Parallel()

	t.Run("returns aggregate counts and average wait duration", func(t *testing.T) {
		t.Parallel()

		var calls int
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Equal(t, []any{"project-1", "env-prod"}, args)
				calls++
				return &mockRow{scanFn: func(dest ...any) error {
					if strings.Contains(sql, "COUNT(*)") {
						*(dest[0].(*int)) = 10
						*(dest[1].(*int)) = 4
						*(dest[2].(*int)) = 3
						*(dest[3].(*int)) = 2
						*(dest[4].(*int)) = 1
						return nil
					}
					*(dest[0].(*float64)) = 12.5
					return nil
				}}
			},
		}

		got, err := New(db).GetEventTriggerStats(context.Background(), "project-1", "env-prod")
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, &EventTriggerStats{
			TotalCount:      10,
			WaitingCount:    4,
			ReceivedCount:   3,
			TimedOutCount:   2,
			CanceledCount:   1,
			AvgWaitDuration: 12.5,
		}, got)
	})

	t.Run("wraps count query errors", func(t *testing.T) {
		t.Parallel()

		countErr := errors.New("count failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return countErr }}
			},
		}

		_, err := New(db).GetEventTriggerStats(context.Background(), "project-1", "")
		require.ErrorContains(t, err, "count event triggers")
		require.ErrorIs(t, err, countErr)
	})

	t.Run("wraps average query errors", func(t *testing.T) {
		t.Parallel()

		var calls int
		avgErr := errors.New("avg failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				return &mockRow{scanFn: func(dest ...any) error {
					if calls == 1 {
						*(dest[0].(*int)) = 1
						*(dest[1].(*int)) = 0
						*(dest[2].(*int)) = 1
						*(dest[3].(*int)) = 0
						*(dest[4].(*int)) = 0
						return nil
					}
					return avgErr
				}}
			},
		}

		_, err := New(db).GetEventTriggerStats(context.Background(), "project-1", "")
		require.ErrorContains(t, err, "avg wait duration")
		require.ErrorIs(t, err, avgErr)
	})
}
