package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// redactBody truncates response bodies in error messages to prevent secret leakage.
func redactBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "...(truncated)"
}

// FlyRuntime implements ContainerRuntime using the Fly Machines API.
type FlyRuntime struct {
	apiToken string
	appName  string
	baseURL  string
	client   *http.Client
}

// NewFlyRuntime creates a new Fly Machines runtime.
func NewFlyRuntime(apiToken, appName string) *FlyRuntime {
	return &FlyRuntime{
		apiToken: apiToken,
		appName:  appName,
		baseURL:  "https://api.machines.dev",
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// WithBaseURL overrides the Fly API base URL (for testing).
func (f *FlyRuntime) WithBaseURL(url string) *FlyRuntime {
	f.baseURL = url
	return f
}

type flyMachineConfig struct {
	Image       string            `json:"image"`
	Guest       flyGuestConfig    `json:"guest"`
	Env         map[string]string `json:"env,omitempty"`
	Restart     flyRestartConfig  `json:"restart"`
	AutoDestroy bool              `json:"auto_destroy"`
}

type flyGuestConfig struct {
	CPUs     int    `json:"cpus"`
	MemoryMB int    `json:"memory_mb"`
	CPUKind  string `json:"cpu_kind"`
}

type flyRestartConfig struct {
	Policy string `json:"policy"`
}

type flyCreateRequest struct {
	Name   string            `json:"name,omitempty"`
	Region string            `json:"region,omitempty"`
	Config flyMachineConfig  `json:"config"`
	Labels map[string]string `json:"metadata,omitempty"`
}

type flyMachineResponse struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Region    string `json:"region"`
	CreatedAt string `json:"created_at"`
}

type flyWaitEvent struct {
	ExitCode int    `json:"exit_code"`
	ExitedAt string `json:"exited_at"`
}

// Run creates a Fly Machine, starts it, waits for exit, and returns the result.
// This is a convenience method that combines Create() + Wait().
func (f *FlyRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	machineID, err := f.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return f.Wait(ctx, machineID, req.TimeoutSecs)
}

// Create provisions and starts a Fly Machine, returning the machine ID.
func (f *FlyRuntime) Create(ctx context.Context, req RunRequest) (string, error) {
	preset, err := PresetFromName(req.MachinePreset)
	if err != nil {
		return "", NewFatalError(422, "invalid machine preset", err)
	}

	createReq := flyCreateRequest{
		Region: req.Region,
		Config: flyMachineConfig{
			Image: req.ImageURI,
			Guest: flyGuestConfig{
				CPUs:     preset.CPUs,
				MemoryMB: preset.MemoryMB,
				CPUKind:  "shared",
			},
			Env:         req.Env,
			Restart:     flyRestartConfig{Policy: "no"},
			AutoDestroy: true,
		},
		Labels: req.Labels,
	}

	if preset.CPUs >= 2 && preset.MemoryMB >= 4096 {
		createReq.Config.Guest.CPUKind = "performance"
	}

	body, _ := json.Marshal(createReq)
	url := fmt.Sprintf("%s/v1/apps/%s/machines", f.baseURL, f.appName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", NewRetryableError(0, "build create request", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+f.apiToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return "", NewRetryableError(0, "create machine request failed", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == 429 {
		return "", NewRetryableError(429, "fly rate limit", nil)
	}
	if resp.StatusCode == 503 {
		return "", NewRetryableError(503, "fly capacity unavailable", nil)
	}
	if resp.StatusCode == 422 {
		return "", NewFatalError(422, fmt.Sprintf("fly config error: %s", redactBody(respBody, 200)), nil)
	}
	if resp.StatusCode >= 500 {
		return "", NewRetryableError(resp.StatusCode, fmt.Sprintf("fly server error: %s", redactBody(respBody, 200)), nil)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", NewRetryableError(resp.StatusCode, fmt.Sprintf("unexpected status: %s", redactBody(respBody, 200)), nil)
	}

	var machine flyMachineResponse
	if err := json.Unmarshal(respBody, &machine); err != nil {
		return "", NewRetryableError(0, "unmarshal create response", err)
	}

	return machine.ID, nil
}

// Wait blocks until a Fly Machine reaches the stopped state, then returns the result.
func (f *FlyRuntime) Wait(ctx context.Context, machineID string, timeoutSecs int) (*RunResult, error) {
	now := time.Now()
	result := &RunResult{
		MachineID: machineID,
		StartedAt: &now,
	}

	waitTimeout := 300
	if timeoutSecs > 0 {
		waitTimeout = timeoutSecs + 30
	}
	waitURL := fmt.Sprintf("%s/v1/apps/%s/machines/%s/wait?timeout=%d&state=stopped", f.baseURL, f.appName, machineID, waitTimeout)

	waitReq, err := http.NewRequestWithContext(ctx, http.MethodGet, waitURL, nil)
	if err != nil {
		return result, NewRetryableError(0, "build wait request", err)
	}
	waitReq.Header.Set("Authorization", "Bearer "+f.apiToken)

	waitResp, err := f.client.Do(waitReq)
	if err != nil {
		return result, NewRetryableError(0, "wait request failed", err)
	}
	defer waitResp.Body.Close()

	finished := time.Now()
	result.FinishedAt = &finished

	if waitResp.StatusCode == 200 {
		status, statusErr := f.getExitEvent(ctx, machineID)
		if statusErr == nil {
			result.ExitCode = status.ExitCode
		}
	}

	return result, nil
}

func (f *FlyRuntime) getExitEvent(ctx context.Context, machineID string) (*flyWaitEvent, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s", f.baseURL, f.appName, machineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build exit event request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Events []struct {
			Type     string `json:"type"`
			ExitCode int    `json:"exit_code"`
		} `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	for _, e := range data.Events {
		if e.Type == "exit" {
			return &flyWaitEvent{ExitCode: e.ExitCode}, nil
		}
	}
	return &flyWaitEvent{ExitCode: -1}, nil
}

// Stop sends a stop signal to a Fly Machine.
func (f *FlyRuntime) Stop(ctx context.Context, machineID string) error {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s/stop", f.baseURL, f.appName, machineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return NewRetryableError(0, "build stop request", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return NewRetryableError(0, "stop machine", err)
	}
	defer resp.Body.Close()
	return nil
}

// Destroy deletes a Fly Machine.
func (f *FlyRuntime) Destroy(ctx context.Context, machineID string) error {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s?force=true", f.baseURL, f.appName, machineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return NewRetryableError(0, "build destroy request", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return NewRetryableError(0, "destroy machine", err)
	}
	defer resp.Body.Close()
	return nil
}

// Status returns the current state of a Fly Machine.
func (f *FlyRuntime) Status(ctx context.Context, machineID string) (MachineStatus, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s", f.baseURL, f.appName, machineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MachineStatusUnknown, fmt.Errorf("build status request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return MachineStatusUnknown, err
	}
	defer resp.Body.Close()

	var machine flyMachineResponse
	if err := json.NewDecoder(resp.Body).Decode(&machine); err != nil {
		return MachineStatusUnknown, err
	}

	switch machine.State {
	case "created":
		return MachineStatusCreated, nil
	case "starting":
		return MachineStatusStarting, nil
	case "started", "running":
		return MachineStatusRunning, nil
	case "stopping":
		return MachineStatusStopping, nil
	case "stopped":
		return MachineStatusStopped, nil
	case "destroyed":
		return MachineStatusDestroyed, nil
	default:
		return MachineStatusUnknown, nil
	}
}

// GetLogs returns the last N lines of container stdout/stderr.
func (f *FlyRuntime) GetLogs(ctx context.Context, machineID string, lines int) (string, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s/logs?limit=%d", f.baseURL, f.appName, machineID, lines)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build logs request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // Cap at 1MB.
	return string(body), nil
}

// Ensure FlyRuntime implements ContainerRuntime.
var _ ContainerRuntime = (*FlyRuntime)(nil)
