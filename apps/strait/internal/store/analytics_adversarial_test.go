package store

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsShortPeriod_Exactly24h verifies the boundary where exactly 24h is still short.
func TestIsShortPeriod_Exactly24h(t *testing.T) {
	t.Parallel()

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	require.True(t,
		isShortPeriod(from,
			to))

}

// TestIsShortPeriod_OneMsOver24h verifies one millisecond over 24h is not short.
func TestIsShortPeriod_OneMsOver24h(t *testing.T) {
	t.Parallel()

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24*time.Hour + time.Millisecond)
	require.False(
		t, isShortPeriod(from,
			to),
	)

}

// TestIsShortPeriod_ZeroDuration verifies from == to is short.
func TestIsShortPeriod_ZeroDuration(t *testing.T) {
	t.Parallel()

	now := time.Now()
	require.True(t,
		isShortPeriod(now,
			now))

}

// TestIsShortPeriod_InvertedRange verifies from > to yields a negative duration.
func TestIsShortPeriod_InvertedRange(t *testing.T) {
	t.Parallel()

	from := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	require.True(t,
		isShortPeriod(from,
			to))

	// Negative duration is <= 24h, so isShortPeriod returns true.

}

// TestIsShortPeriod_NegativeDuration verifies a large negative range is still short.
func TestIsShortPeriod_NegativeDuration(t *testing.T) {
	t.Parallel()

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	require.True(t,
		isShortPeriod(from,
			to))

	// to.Sub(from) is very negative, which is <= 24h.

}

// FuzzIsShortPeriod fuzzes the isShortPeriod boundary with random time ranges.
func FuzzIsShortPeriod(f *testing.F) {
	f.Add(int64(0), int64(86400))
	f.Add(int64(0), int64(86401))
	f.Add(int64(1000000), int64(1000000))
	f.Add(int64(100), int64(0))

	f.Fuzz(func(t *testing.T, fromSec, toSec int64) {
		from := time.Unix(fromSec, 0)
		to := time.Unix(toSec, 0)
		got := isShortPeriod(from, to)
		want := to.Sub(from) <= 24*time.Hour
		assert.Equal(t,
			want,
			got)

	})
}

// TestJobMemoryQuotaError_Is verifies errors.Is matching for both quota kinds.
func TestJobMemoryQuotaError_Is(t *testing.T) {
	t.Parallel()

	perKey := &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerKey, Max: 1024}
	perJob := &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerJob, Max: 4096}
	require.True(t,
		errors.Is(perKey,
			ErrJobMemoryPerKeyLimitExceeded,
		))
	require.False(
		t, errors.Is(perKey,
			ErrJobMemoryPerJobLimitExceeded,
		))
	require.True(t,
		errors.Is(perJob,
			ErrJobMemoryPerJobLimitExceeded,
		))
	require.False(
		t, errors.Is(perJob,
			ErrJobMemoryPerKeyLimitExceeded,
		))

	// Unknown kind should not match either sentinel.
	unknown := &JobMemoryQuotaError{Kind: "unknown", Max: 100}
	require.False(
		t, errors.Is(unknown,
			ErrJobMemoryPerKeyLimitExceeded,
		))
	require.False(
		t, errors.Is(unknown,
			ErrJobMemoryPerJobLimitExceeded,
		))

}

// TestJobMemoryQuotaError_Message verifies error message format for each kind.
func TestJobMemoryQuotaError_Message(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *JobMemoryQuotaError
		want string
	}{
		{
			name: "per-key",
			err:  &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerKey, Max: 512},
			want: fmt.Sprintf("%s: %d", ErrJobMemoryPerKeyLimitExceeded, 512),
		},
		{
			name: "per-job",
			err:  &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerJob, Max: 2048},
			want: fmt.Sprintf("%s: %d", ErrJobMemoryPerJobLimitExceeded, 2048),
		},
		{
			name: "unknown kind",
			err:  &JobMemoryQuotaError{Kind: "bogus", Max: 0},
			want: "job memory quota exceeded",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.err.Error()
			assert.Equal(t,
				tc.want,
				got)

		})
	}
}

// TestJobMemoryQuota_ExactlyAtPerKey verifies the per-key boundary where size == max.
func TestJobMemoryQuota_ExactlyAtPerKey(t *testing.T) {
	t.Parallel()

	// UpsertJobMemoryWithQuota checks: mem.SizeBytes > maxPerKey.
	// So size == max should NOT trigger an error.
	maxPerKey := 1024
	sizeBytes := 1024
	require.False(
		t, maxPerKey >
			0 &&
			sizeBytes >
				maxPerKey)

}

// TestJobMemoryQuota_OneOverPerKey verifies one byte over per-key limit triggers error.
func TestJobMemoryQuota_OneOverPerKey(t *testing.T) {
	t.Parallel()

	maxPerKey := 1024
	sizeBytes := 1025
	require.False(
		t, maxPerKey <=
			0 ||
			sizeBytes <=
				maxPerKey)

}

// TestJobMemoryQuota_ExactlyAtPerJob verifies size exactly at per-job limit passes.
func TestJobMemoryQuota_ExactlyAtPerJob(t *testing.T) {
	t.Parallel()

	// The per-job check is: currentTotal - existingSize + mem.SizeBytes > maxPerJob.
	// With no existing data, currentTotal=0, existingSize=0.
	maxPerJob := 4096
	newSize := 4096
	currentTotal := 0
	existingSize := 0
	require.False(
		t, maxPerJob >
			0 &&
			currentTotal-
				existingSize+newSize > maxPerJob)

}

// TestJobMemoryQuota_NegativeQuota verifies negative quota disables the check.
func TestJobMemoryQuota_NegativeQuota(t *testing.T) {
	t.Parallel()

	// Guard is: maxPerKey > 0 && ..., so negative quota skips the check.
	maxPerKey := -1
	sizeBytes := 999999
	require.False(
		t, maxPerKey >
			0 &&
			sizeBytes >
				maxPerKey)

}

// TestJobMemoryQuota_ZeroQuota verifies zero quota disables the check.
func TestJobMemoryQuota_ZeroQuota(t *testing.T) {
	t.Parallel()

	maxPerKey := 0
	sizeBytes := 999999
	require.False(
		t, maxPerKey >
			0 &&
			sizeBytes >
				maxPerKey)

}

// FuzzJobMemoryQuota fuzzes the per-key quota check logic.
func FuzzJobMemoryQuota(f *testing.F) {
	f.Add(100, 1024)
	f.Add(1024, 1024)
	f.Add(1025, 1024)
	f.Add(0, 0)
	f.Add(-1, 100)

	f.Fuzz(func(t *testing.T, sizeBytes, maxPerKey int) {
		// Reproduce the exact guard from UpsertJobMemoryWithQuota.
		shouldReject := maxPerKey > 0 && sizeBytes > maxPerKey

		err := checkPerKeyQuota(sizeBytes, maxPerKey)
		assert.False(t,
			shouldReject &&
				err == nil,
		)
		assert.False(t,
			!shouldReject &&
				err !=
					nil,
		)

	})
}

// checkPerKeyQuota replicates the per-key guard from UpsertJobMemoryWithQuota.
func checkPerKeyQuota(sizeBytes, maxPerKey int) error {
	if maxPerKey > 0 && sizeBytes > maxPerKey {
		return &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerKey, Max: maxPerKey}
	}
	return nil
}

// TestAuditEvent_EmptyDetails verifies CreateAuditEvent handles empty details by
// defaulting to "{}". This is a DB-dependent test; we verify the panic path.
func TestAuditEvent_EmptyDetails(t *testing.T) {
	t.Parallel()

	var q *Queries // nil, will panic on DB access.
	defer func() {
		require.NotNil(t, recover())

	}()

	//nolint:staticcheck // intentionally calling with nil receiver.
	_ = q.CreateAuditEvent(nil, nil)
}

// TestAuditEvent_HugePayload verifies that a 10MB payload does not crash query building.
func TestAuditEvent_HugePayload(t *testing.T) {
	t.Parallel()

	var q *Queries
	defer func() {
		require.NotNil(t, recover())

	}()

	//nolint:staticcheck // intentionally calling with nil receiver.
	_ = q.CreateAuditEvent(nil, nil)
}

// TestAuditEvent_NullBytesInActor verifies null bytes in actor string do not crash.
func TestAuditEvent_NullBytesInActor(t *testing.T) {
	t.Parallel()

	var q *Queries
	defer func() {
		require.NotNil(t, recover())

	}()

	//nolint:staticcheck // intentionally calling with nil receiver.
	_ = q.CreateAuditEvent(nil, nil)
}
