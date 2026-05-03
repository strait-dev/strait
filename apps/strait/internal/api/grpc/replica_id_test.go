package grpc

import (
	"os"
	"sync"
	"testing"
)

// TestReplicaID_UsesHOSTNAME verifies that HOSTNAME env var is preferred.
// Note: sync.Once is process-global, so we reset internal state via a helper.
func TestReplicaID_UsesHOSTNAME(t *testing.T) {
	// Reset the once so this test can observe the HOSTNAME env var.
	// We do this by direct field reset — acceptable in package-internal tests.
	replicaIDOnce = sync.Once{}
	replicaID = ""

	t.Setenv("HOSTNAME", "test-pod-abc")

	id := ReplicaID()
	if id != "test-pod-abc" {
		t.Errorf("expected test-pod-abc, got %s", id)
	}

	// Reset again for subsequent tests.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}

// TestReplicaID_FallbackUUID verifies that when HOSTNAME is unset, a non-empty UUID is generated.
func TestReplicaID_FallbackUUID(t *testing.T) {
	// Reset the once so we start fresh.
	replicaIDOnce = sync.Once{}
	replicaID = ""

	os.Unsetenv("HOSTNAME")

	id := ReplicaID()
	if id == "" {
		t.Error("expected non-empty UUID fallback, got empty string")
	}

	// Reset for subsequent tests.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}

// TestReplicaID_Stable verifies that calling ReplicaID() multiple times returns the same value.
func TestReplicaID_Stable(t *testing.T) {
	// Reset so we get a fresh UUID.
	replicaIDOnce = sync.Once{}
	replicaID = ""

	os.Unsetenv("HOSTNAME")

	id1 := ReplicaID()
	id2 := ReplicaID()
	if id1 != id2 {
		t.Errorf("ReplicaID not stable across calls: %q != %q", id1, id2)
	}

	// Reset after test.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}

// TestReplicaID_NonEmptyAlways verifies the invariant that ReplicaID is always non-empty.
func TestReplicaID_NonEmptyAlways(t *testing.T) {
	replicaIDOnce = sync.Once{}
	replicaID = ""

	// Test with HOSTNAME set.
	t.Setenv("HOSTNAME", "k8s-node-1")
	id := ReplicaID()
	if id == "" {
		t.Error("ReplicaID must never return empty string")
	}

	// Reset and test without HOSTNAME.
	replicaIDOnce = sync.Once{}
	replicaID = ""
	os.Unsetenv("HOSTNAME")

	id = ReplicaID()
	if id == "" {
		t.Error("ReplicaID must never return empty string (fallback path)")
	}

	// Cleanup.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}
