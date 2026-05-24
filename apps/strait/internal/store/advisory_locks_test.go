package store

import (
	"context"
	"strings"
	"testing"
)

func TestAcquireAdvisoryLock_RejectsNilQueries(t *testing.T) {
	t.Parallel()
	err := AcquireAdvisoryLock(context.Background(), nil, AdvisoryLockNsAuditChain, "proj-1")
	if err == nil {
		t.Fatal("expected error for nil *Queries, got nil")
	}
	if !strings.Contains(err.Error(), "queries is nil") {
		t.Errorf("error = %q, expected mention of nil queries", err)
	}
}

func TestAcquireAdvisoryLock_RejectsEmptyNamespace(t *testing.T) {
	t.Parallel()
	q := New(nil)
	err := AcquireAdvisoryLock(context.Background(), q, "", "proj-1")
	if err == nil {
		t.Fatal("expected error for empty namespace, got nil")
	}
	if !strings.Contains(err.Error(), "namespace is empty") {
		t.Errorf("error = %q, expected mention of empty namespace", err)
	}
}

func TestAcquireAdvisoryLock_RejectsEmptyKey(t *testing.T) {
	t.Parallel()
	q := New(nil)
	err := AcquireAdvisoryLock(context.Background(), q, AdvisoryLockNsAuditChain, "")
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
	if !strings.Contains(err.Error(), "key is empty") {
		t.Errorf("error = %q, expected mention of empty key", err)
	}
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
	// All constants must be distinct string literals. Map insertion
	// deduplicates, so a collision would show up as len < 3.
	if len(namespaces) != 3 {
		t.Fatalf("advisory lock namespaces collided: %v", namespaces)
	}
	for ns := range namespaces {
		if ns == "" {
			t.Error("advisory lock namespace is empty — must be a non-empty literal")
		}
		if !strings.HasSuffix(ns, ":") {
			t.Errorf("namespace %q does not end with ':' — all namespaces must be prefix-safe", ns)
		}
	}
}
