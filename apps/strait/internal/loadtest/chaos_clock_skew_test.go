//go:build loadtest

package loadtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClockSkewVerdict locks in that the clock-skew chaos scenario fails when
// fewer than the inserted future-dated rows survive the reaper soak. Previously
// the survivor count was scanned but never compared, so the scenario passed even
// if the reaper wrongly deleted not-yet-due runs, making the stated invariant a
// no-op.
func TestClockSkewVerdict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		remaining int
		inserted  int
		wantErr   bool
	}{
		{name: "all survive", remaining: 100, inserted: 100, wantErr: false},
		{name: "more than inserted", remaining: 120, inserted: 100, wantErr: false},
		{name: "one reaped", remaining: 99, inserted: 100, wantErr: true},
		{name: "all reaped", remaining: 0, inserted: 100, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := clockSkewVerdict(tc.remaining, tc.inserted)
			require.False(t, tc.
				wantErr && err ==

				nil)
			require.False(t, !tc.
				wantErr && err !=
				nil)

		})
	}
}
