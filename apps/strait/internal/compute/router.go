package compute

import (
	"context"
	"log/slog"
	"sync"
)

// RuntimeRouter implements ContainerRuntime with primary/fallback routing.
// It tracks which runtime created each machine to route subsequent operations correctly.
type RuntimeRouter struct {
	primary  ContainerRuntime
	fallback ContainerRuntime
	owners   sync.Map // machineID -> ContainerRuntime
}

// NewRuntimeRouter creates a router with a primary and optional fallback runtime.
func NewRuntimeRouter(primary, fallback ContainerRuntime) *RuntimeRouter {
	return &RuntimeRouter{
		primary:  primary,
		fallback: fallback,
	}
}

// Run delegates to the primary runtime; falls back on retryable errors.
func (r *RuntimeRouter) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	machineID, err := r.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return r.Wait(ctx, machineID, req.TimeoutSecs)
}

// Create tries the primary runtime first; on retryable error, falls back.
func (r *RuntimeRouter) Create(ctx context.Context, req RunRequest) (string, error) {
	id, err := r.primary.Create(ctx, req)
	if err == nil {
		r.owners.Store(id, r.primary)
		return id, nil
	}

	if r.fallback == nil || !IsRetryable(err) {
		return "", err
	}

	slog.Warn("compute primary failed, falling back",
		"error", err,
		"primary", runtimeName(r.primary),
		"fallback", runtimeName(r.fallback),
	)

	id, fallbackErr := r.fallback.Create(ctx, req)
	if fallbackErr != nil {
		return "", fallbackErr
	}
	r.owners.Store(id, r.fallback)
	return id, nil
}

// Wait routes to the runtime that owns the machineID.
// Does NOT delete the owner -- Destroy or Stop handles cleanup so that
// post-Wait calls (GetLogs, Status) still route to the correct runtime.
func (r *RuntimeRouter) Wait(ctx context.Context, machineID string, timeoutSecs int) (*RunResult, error) {
	return r.owner(machineID).Wait(ctx, machineID, timeoutSecs)
}

// Stop routes to the runtime that owns the machineID.
func (r *RuntimeRouter) Stop(ctx context.Context, machineID string) error {
	err := r.owner(machineID).Stop(ctx, machineID)
	r.owners.Delete(machineID)
	return err
}

// Start routes to the runtime that owns the machineID.
func (r *RuntimeRouter) Start(ctx context.Context, machineID string, env map[string]string) error {
	return r.owner(machineID).Start(ctx, machineID, env)
}

// Destroy routes to the runtime that owns the machineID and cleans up tracking.
func (r *RuntimeRouter) Destroy(ctx context.Context, machineID string) error {
	err := r.owner(machineID).Destroy(ctx, machineID)
	r.owners.Delete(machineID)
	return err
}

// Status routes to the runtime that owns the machineID.
func (r *RuntimeRouter) Status(ctx context.Context, machineID string) (MachineStatus, error) {
	return r.owner(machineID).Status(ctx, machineID)
}

// GetLogs routes to the runtime that owns the machineID.
func (r *RuntimeRouter) GetLogs(ctx context.Context, machineID string, lines int) (string, error) {
	return r.owner(machineID).GetLogs(ctx, machineID, lines)
}

// owner returns the runtime that created the given machineID, defaulting to primary.
func (r *RuntimeRouter) owner(machineID string) ContainerRuntime {
	if rt, ok := r.owners.Load(machineID); ok {
		return rt.(ContainerRuntime)
	}
	return r.primary
}

// runtimeName returns a human-readable name for logging.
func runtimeName(rt ContainerRuntime) string {
	switch rt.(type) {
	case *FlyRuntime:
		return "fly"
	case *K8sRuntime:
		return "k8s"
	default:
		return "unknown"
	}
}

// Ensure RuntimeRouter implements ContainerRuntime.
var _ ContainerRuntime = (*RuntimeRouter)(nil)
