package grpc

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, "test-pod-abc",

		id)

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
	assert.NotEqual(t,
		"", id)

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
	assert.Equal(t, id2,
		id1)

	// Reset after test.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}

// TestReplicaID_NonEmptyAlways verifies the invariant that ReplicaID is always non-empty.
func TestReplicaID_NonEmptyAlways(t *testing.T) {
	replicaIDOnce = sync.Once{}
	replicaID = ""

	// Test with HOSTNAME set.
	t.Setenv("HOSTNAME", "container-node-1")
	id := ReplicaID()
	assert.NotEqual(t,
		"", id)

	// Reset and test without HOSTNAME.
	replicaIDOnce = sync.Once{}
	replicaID = ""
	os.Unsetenv("HOSTNAME")

	id = ReplicaID()
	assert.NotEqual(t,
		"", id)

	// Cleanup.
	replicaIDOnce = sync.Once{}
	replicaID = ""
}
