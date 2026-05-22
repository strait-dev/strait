package grpc

import (
	"context"
	"encoding/json"

	"strait/internal/domain"
)

// WorkerRunResultFinalizer owns the authoritative run-state transition for
// worker-mode results received outside the normal WorkerDispatch wait path.
type WorkerRunResultFinalizer interface {
	FinalizeWorkerRunResult(ctx context.Context, runID, status, errorMessage string, output json.RawMessage) (domain.WorkerTaskStatus, error)
}
