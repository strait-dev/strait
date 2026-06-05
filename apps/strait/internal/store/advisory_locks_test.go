package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireAdvisoryLock_RejectsNilQueries(t *testing.T) {
	t.Parallel()
	err := AcquireAdvisoryLock(context.Background(), nil, AdvisoryLockNsAuditChain, "proj-1")
	require.Error(t,
		err)
	assert.True(t,
		strings.Contains(err.Error(), "queries is nil"))

}

func TestAcquireAdvisoryLock_RejectsEmptyNamespace(t *testing.T) {
	t.Parallel()
	q := New(nil)
	err := AcquireAdvisoryLock(context.Background(), q, "", "proj-1")
	require.Error(t,
		err)
	assert.True(t,
		strings.Contains(err.Error(), "namespace is empty"))

}

func TestAcquireAdvisoryLock_RejectsEmptyKey(t *testing.T) {
	t.Parallel()
	q := New(nil)
	err := AcquireAdvisoryLock(context.Background(), q, AdvisoryLockNsAuditChain, "")
	require.Error(t,
		err)
	assert.True(t,
		strings.Contains(err.Error(), "key is empty"))

}

// TestAdvisoryLockNamespaces_Distinct guards against two namespaces
// accidentally aliasing to the same literal. Any future addition of a
// namespace constant that equals an existing one would silently lose
// the intended serialization domain — this test fails loudly instead.
func TestAdvisoryLockNamespaces_Distinct(t *testing.T) {
	t.Parallel()
	namespaces := map[string]struct{}{
		AdvisoryLockNsAuditChain:      {},
		AdvisoryLockNsAuditChainShard: {},
		AdvisoryLockNsAuditRotate:     {},
	}
	require.Len(t,
		namespaces,
		3)

	// All constants must be distinct string literals. Map insertion
	// deduplicates, so a collision would show up as len < 3.

	for ns := range namespaces {
		assert.NotEqual(t, "", ns)
		assert.True(t,
			strings.HasSuffix(ns,
				":",
			))

	}
}
