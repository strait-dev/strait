package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type benchmarkCacheKey struct {
	ProjectID string
	JobID     string
}

func TestTierSingleflightKey(t *testing.T) {
	t.Parallel()

	require.Equal(t, "worker_job:job-123:42", tierSingleflightKey("worker_job", "job-123", 42, false))
	require.Equal(t, "api_key:key-hash:99:versioned", tierSingleflightKey("api_key", "key-hash", 99, true))
	require.Equal(t, "deps:{proj-1 job-1}:7", tierSingleflightKey("deps", benchmarkCacheKey{ProjectID: "proj-1", JobID: "job-1"}, 7, false))
}

func BenchmarkTierSingleflightKeyString(b *testing.B) {
	for b.Loop() {
		_ = tierSingleflightKey("worker_job", "job-123456789", 123456789, false)
	}
}

func BenchmarkTierSingleflightKeyStringVersioned(b *testing.B) {
	for b.Loop() {
		_ = tierSingleflightKey("api_key", "key-hash-123456789", 123456789, true)
	}
}

func BenchmarkTierSingleflightKeyStruct(b *testing.B) {
	key := benchmarkCacheKey{ProjectID: "proj-123456789", JobID: "job-123456789"}
	for b.Loop() {
		_ = tierSingleflightKey("deps", key, 123456789, false)
	}
}
