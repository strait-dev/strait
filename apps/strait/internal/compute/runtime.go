// Package compute provides container runtime abstractions for managed job execution.
package compute

import (
	"context"
	"time"
)

// ContainerRuntime is the interface that all container backends must implement.
type ContainerRuntime interface {
	// Run provisions a container, waits for it to exit, and returns the result.
	Run(ctx context.Context, req RunRequest) (*RunResult, error)

	// Stop sends a stop signal to a running container.
	Stop(ctx context.Context, machineID string) error

	// Destroy deletes a container and its resources.
	Destroy(ctx context.Context, machineID string) error

	// Status returns the current state of a container.
	Status(ctx context.Context, machineID string) (MachineStatus, error)

	// GetLogs returns the last N lines of a container's stdout/stderr.
	GetLogs(ctx context.Context, machineID string, lines int) (string, error)
}

// RunRequest describes the container to provision and run.
type RunRequest struct {
	ImageURI      string            // Required: container image to run.
	MachinePreset string            // Required: compute tier (micro, small-1x, etc.).
	Region        string            // Optional: deployment region (defaults to config).
	Env           map[string]string // Environment variables to inject.
	Labels        map[string]string // Metadata labels for tracking.
	TimeoutSecs   int               // Maximum wall-clock seconds before kill.
}

// RunResult captures the outcome of a container execution.
type RunResult struct {
	MachineID  string     // Provider-assigned container/machine ID.
	ExitCode   int        // Process exit code (0 = success).
	StartedAt  *time.Time // When the container started running.
	FinishedAt *time.Time // When the container exited.
	Logs       string     // Last N lines of stdout/stderr (on crash).
}

// MachineStatus represents the lifecycle state of a container.
type MachineStatus string

const (
	MachineStatusCreated   MachineStatus = "created"
	MachineStatusStarting  MachineStatus = "starting"
	MachineStatusRunning   MachineStatus = "running"
	MachineStatusStopping  MachineStatus = "stopping"
	MachineStatusStopped   MachineStatus = "stopped"
	MachineStatusDestroyed MachineStatus = "destroyed"
	MachineStatusUnknown   MachineStatus = "unknown"
)
