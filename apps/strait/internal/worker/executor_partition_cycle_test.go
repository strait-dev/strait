package worker

import (
	"testing"

	"strait/internal/testutil"
)

func TestBuildPartitionCycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		partitions []string
		weights    string
		want       []string
	}{
		{
			name:       "weights repeat matching partitions",
			partitions: []string{"proj-a", "proj-b"},
			weights:    "proj-a:2,proj-b:1",
			want:       []string{"proj-a", "proj-a", "proj-b"},
		},
		{
			name:       "missing weights default to one",
			partitions: []string{"proj-a", "proj-b", "proj-c"},
			weights:    "proj-a:2",
			want:       []string{"proj-a", "proj-a", "proj-b", "proj-c"},
		},
		{
			name:       "malformed weights are ignored",
			partitions: []string{"proj-a", "proj-b"},
			weights:    "proj-a:bad,proj-b:-1,invalid,proj-a:3",
			want:       []string{"proj-a", "proj-a", "proj-a", "proj-b"},
		},
		{
			name:       "empty partitions disables partitioning",
			partitions: nil,
			weights:    "proj-a:2",
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildPartitionCycle(tt.partitions, tt.weights)
			testutil.AssertEqual(t, got, tt.want)
		})
	}
}
