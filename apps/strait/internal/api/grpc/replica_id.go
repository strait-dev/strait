package grpc

import (
	"os"
	"sync"

	"github.com/google/uuid"
)

var (
	replicaIDOnce sync.Once
	replicaID     string
)

// ReplicaID returns a stable identifier for this server replica.
// It prefers the HOSTNAME env var (set by Kubernetes as the pod name) and
// falls back to a process-local UUID generated once at startup.
func ReplicaID() string {
	replicaIDOnce.Do(func() {
		if h := os.Getenv("HOSTNAME"); h != "" {
			replicaID = h
			return
		}
		replicaID = uuid.NewString()
	})
	return replicaID
}
