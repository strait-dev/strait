package compute

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DockerRuntime implements ContainerRuntime using the local Docker Engine.
// Intended for development and testing, not production.
type DockerRuntime struct{}

// NewDockerRuntime creates a new Docker runtime for dev/test use.
func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{}
}

// Run creates and runs a Docker container, waits for it to exit, and returns the result.
func (d *DockerRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.ImageURI == "" {
		return nil, NewFatalError(422, "image_uri is required", nil)
	}

	containerName := fmt.Sprintf("strait-%s", uuid.Must(uuid.NewV7()).String()[:8])

	args := []string{"run", "--name", containerName, "--rm"}

	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}

	for k, v := range req.Labels {
		args = append(args, "--label", k+"="+v)
	}

	if req.TimeoutSecs > 0 {
		args = append(args, "--stop-timeout", strconv.Itoa(req.TimeoutSecs))
	}

	args = append(args, req.ImageURI)

	now := time.Now()
	result := &RunResult{
		MachineID: containerName,
		StartedAt: &now,
	}

	timeoutCtx := ctx
	if req.TimeoutSecs > 0 {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSecs)*time.Second+30*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(timeoutCtx, "docker", args...) //nolint:gosec // Args are constructed from trusted input (image URI, env vars).
	output, err := cmd.CombinedOutput()

	finished := time.Now()
	result.FinishedAt = &finished
	result.Logs = string(output)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		if strings.Contains(err.Error(), "signal: killed") || ctx.Err() != nil {
			result.ExitCode = 137
			return result, nil
		}
		return result, NewRetryableError(0, "docker run failed", err)
	}

	result.ExitCode = 0
	return result, nil
}

// Stop sends a stop signal to a Docker container.
func (d *DockerRuntime) Stop(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName) //nolint:gosec // Container name from internal state.
	return cmd.Run()
}

// Destroy removes a Docker container.
func (d *DockerRuntime) Destroy(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName) //nolint:gosec // Container name from internal state.
	return cmd.Run()
}

// Status returns the current state of a Docker container.
func (d *DockerRuntime) Status(ctx context.Context, containerName string) (MachineStatus, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", containerName) //nolint:gosec // Container name from internal state.
	output, err := cmd.Output()
	if err != nil {
		return MachineStatusUnknown, err
	}
	state := strings.TrimSpace(string(output))
	switch state {
	case "created":
		return MachineStatusCreated, nil
	case "running":
		return MachineStatusRunning, nil
	case "exited", "dead":
		return MachineStatusStopped, nil
	default:
		return MachineStatusUnknown, nil
	}
}

// GetLogs returns the last N lines of container logs.
func (d *DockerRuntime) GetLogs(ctx context.Context, containerName string, lines int) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", strconv.Itoa(lines), containerName) //nolint:gosec // Container name from internal state.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// Ensure DockerRuntime implements ContainerRuntime.
var _ ContainerRuntime = (*DockerRuntime)(nil)
