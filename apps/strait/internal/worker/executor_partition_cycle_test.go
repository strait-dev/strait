package worker

import (
	"fmt"
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

func BenchmarkBuildPartitionCycle(b *testing.B) {
	partitions := []string{"proj-a", "proj-b", "proj-c", "proj-d"}

	benchmarks := []struct {
		name    string
		weights string
		wantLen int
	}{
		{name: "empty_weights", weights: "", wantLen: 4},
		{name: "configured_weights", weights: "proj-a:4, proj-b:2, proj-c:1, proj-d:3", wantLen: 10},
		{name: "mixed_malformed", weights: "proj-a:bad, proj-b:-1, invalid, proj-c:3, proj-d:2", wantLen: 7},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				got := buildPartitionCycle(partitions, bm.weights)
				if len(got) != bm.wantLen {
					b.Fatalf("cycle length = %d, want %d", len(got), bm.wantLen)
				}
			}
		})
	}
}

func BenchmarkParsePartitionWeights(b *testing.B) {
	benchmarks := []string{
		"",
		"proj-a:4, proj-b:2, proj-c:1, proj-d:3",
		"proj-a:bad, proj-b:-1, invalid, proj-c:3, proj-d:2",
	}

	for _, weights := range benchmarks {
		b.Run(fmt.Sprintf("len_%d", len(weights)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = parsePartitionWeights(weights)
			}
		})
	}
}
