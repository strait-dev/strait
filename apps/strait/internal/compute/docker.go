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

// validateImageURI rejects image URIs with shell metacharacters.
func validateImageURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("image URI is required")
	}
	for _, c := range uri {
		if !isDockerSafeChar(c) {
			return fmt.Errorf("image URI contains invalid character: %q", c)
		}
	}

	// Reject embedded credentials (user:pass@registry).
	// Allow @sha256: for digest pinning.
	if atIdx := strings.Index(uri, "@"); atIdx >= 0 {
		after := uri[atIdx:]
		if !strings.HasPrefix(after, "@sha256:") {
			return fmt.Errorf("image URI must not contain embedded credentials; use @sha256: for digest pinning only")
		}
	}

	return nil
}

// extractRegistry parses the registry hostname from a Docker image URI.
// For "docker.io/library/nginx:latest" returns "docker.io".
// For "nginx:latest" (no explicit registry) returns "docker.io" (Docker Hub default).
func extractRegistry(uri string) string {
	// Remove tag/digest.
	base := uri
	if atIdx := strings.Index(base, "@"); atIdx >= 0 {
		base = base[:atIdx]
	}
	if colonIdx := strings.LastIndex(base, ":"); colonIdx >= 0 {
		// Only strip if it looks like a tag (no slash after colon = tag, slash after = port).
		afterColon := base[colonIdx+1:]
		if !strings.Contains(afterColon, "/") {
			base = base[:colonIdx]
		}
	}

	// Split on first /.
	parts := strings.SplitN(base, "/", 2)
	if len(parts) == 1 {
		return "docker.io" // no registry specified = Docker Hub
	}

	registry := parts[0]
	// If registry doesn't contain a dot or colon, it's a Docker Hub user/org.
	if !strings.Contains(registry, ".") && !strings.Contains(registry, ":") {
		return "docker.io"
	}
	return registry
}

// ValidateImageRegistry checks if the image URI's registry is in the allowlist.
// An empty allowlist means no restriction (all registries allowed).
func ValidateImageRegistry(uri string, allowedRegistries []string) error {
	if len(allowedRegistries) == 0 {
		return nil
	}
	registry := extractRegistry(uri)
	for _, allowed := range allowedRegistries {
		if matchesRegistry(allowed, strings.ToLower(registry)) {
			return nil
		}
	}
	return fmt.Errorf("image registry %q not in allowlist", registry)
}

func matchesRegistry(pattern, registry string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == registry {
		return true
	}
	// Support wildcard prefix: *.ecr.amazonaws.com.
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".ecr.amazonaws.com"
		return strings.HasSuffix(registry, suffix)
	}
	return false
}

// ValidateImageDigest checks that the image URI contains a digest pin.
func ValidateImageDigest(uri string) error {
	if !strings.Contains(uri, "@sha256:") {
		return fmt.Errorf("image must use digest pinning (@sha256:...)")
	}
	return nil
}

func isDockerSafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' ||
		c == ':' || c == '/' || c == '@'
}

// validateEnvKey rejects env var keys with special characters.
func validateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("env key must not be empty")
	}
	for _, c := range key {
		if !isEnvKeySafeChar(c) {
			return fmt.Errorf("env key contains invalid character: %q", c)
		}
	}
	return nil
}

func isEnvKeySafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// validateLabelKey rejects label keys with shell metacharacters.
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key must not be empty")
	}
	for _, c := range key {
		if !isLabelKeySafeChar(c) {
			return fmt.Errorf("label key contains invalid character: %q", c)
		}
	}
	return nil
}

func isLabelKeySafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_'
}

// DockerRuntime implements ContainerRuntime using the local Docker Engine.
// Intended for development and testing, not production.
type DockerRuntime struct{}

// NewDockerRuntime creates a new Docker runtime for dev/test use.
func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{}
}

// Create starts a Docker container and returns its name. Unlike Fly, Docker
// containers created here use `docker run -d` (detached) so the caller can
// separately Wait().
func (d *DockerRuntime) Create(ctx context.Context, req RunRequest) (string, error) {
	if err := validateImageURI(req.ImageURI); err != nil {
		return "", NewFatalError(422, err.Error(), nil)
	}
	for k := range req.Env {
		if err := validateEnvKey(k); err != nil {
			return "", NewFatalError(422, err.Error(), nil)
		}
	}
	for k := range req.Labels {
		if err := validateLabelKey(k); err != nil {
			return "", NewFatalError(422, err.Error(), nil)
		}
	}

	containerName := fmt.Sprintf("strait-%s", uuid.Must(uuid.NewV7()).String()[:8])

	args := []string{"run", "-d", "--name", containerName}

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

	cmd := exec.CommandContext(ctx, "docker", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", NewRetryableError(0, fmt.Sprintf("docker create failed: %s", string(output)), err)
	}

	return containerName, nil
}

// Wait blocks until a Docker container exits and returns the result.
func (d *DockerRuntime) Wait(ctx context.Context, containerName string, timeoutSecs int) (*RunResult, error) {
	now := time.Now()
	result := &RunResult{
		MachineID: containerName,
		StartedAt: &now,
	}

	waitCtx := ctx
	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second+30*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(waitCtx, "docker", "wait", containerName)
	output, err := cmd.Output()

	finished := time.Now()
	result.FinishedAt = &finished

	if err != nil {
		result.ExitCode = 137
		return result, nil
	}

	code, parseErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if parseErr != nil {
		result.ExitCode = -1
	} else {
		result.ExitCode = code
	}

	// Fetch logs for crash diagnosis.
	if result.ExitCode != 0 {
		if logs, logErr := d.GetLogs(ctx, containerName, 100); logErr == nil {
			result.Logs = logs
		}
	}

	return result, nil
}

// Run creates and runs a Docker container, waits for it to exit, and returns the result.
func (d *DockerRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if err := validateImageURI(req.ImageURI); err != nil {
		return nil, NewFatalError(422, err.Error(), nil)
	}
	for k := range req.Env {
		if err := validateEnvKey(k); err != nil {
			return nil, NewFatalError(422, err.Error(), nil)
		}
	}
	for k := range req.Labels {
		if err := validateLabelKey(k); err != nil {
			return nil, NewFatalError(422, err.Error(), nil)
		}
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

	cmd := exec.CommandContext(timeoutCtx, "docker", args...)
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

// Start is not supported for the Docker dev runtime. Always returns ErrMachineGone.
func (d *DockerRuntime) Start(_ context.Context, _ string, _ map[string]string) error {
	return ErrMachineGone
}

// Stop sends a stop signal to a Docker container.
func (d *DockerRuntime) Stop(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	return cmd.Run()
}

// Destroy removes a Docker container.
func (d *DockerRuntime) Destroy(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
	return cmd.Run()
}

// Status returns the current state of a Docker container.
func (d *DockerRuntime) Status(ctx context.Context, containerName string) (MachineStatus, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", containerName)
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
