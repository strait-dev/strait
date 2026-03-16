package compute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFlyRuntime_Run_CreatesWithCorrectConfig(t *testing.T) {
	t.Parallel()

	var receivedBody flyCreateRequest
	var receivedAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/apps/my-app/machines", func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-123", State: "started"})
	})
	mux.HandleFunc("GET /v1/apps/my-app/machines/m-123/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/my-app/machines/m-123", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "m-123",
			"state": "stopped",
			"events": []map[string]any{
				{"type": "exit", "exit_code": 0},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("fly-token-123", "my-app").WithBaseURL(srv.URL)
	result, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "registry.example.com/app:latest",
		MachinePreset: "small-1x",
		Region:        "ewr",
		Env:           map[string]string{"RUN_ID": "run-1", "SDK_TOKEN": "tok"},
		Labels:        map[string]string{"job_id": "j-1", "project_id": "p-1"},
		TimeoutSecs:   60,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify auth header.
	if receivedAuth != "Bearer fly-token-123" {
		t.Errorf("auth header = %q, want Bearer fly-token-123", receivedAuth)
	}

	// Verify request body.
	if receivedBody.Config.Image != "registry.example.com/app:latest" {
		t.Errorf("image = %q", receivedBody.Config.Image)
	}
	if receivedBody.Region != "ewr" {
		t.Errorf("region = %q, want ewr", receivedBody.Region)
	}
	if receivedBody.Config.Restart.Policy != "no" {
		t.Errorf("restart policy = %q, want no", receivedBody.Config.Restart.Policy)
	}
	if !receivedBody.Config.AutoDestroy {
		t.Error("auto_destroy should be true")
	}
	if receivedBody.Config.Env["RUN_ID"] != "run-1" {
		t.Errorf("env RUN_ID = %q", receivedBody.Config.Env["RUN_ID"])
	}
	if receivedBody.Labels["job_id"] != "j-1" {
		t.Errorf("label job_id = %q", receivedBody.Labels["job_id"])
	}

	// Verify result.
	if result.MachineID != "m-123" {
		t.Errorf("MachineID = %q, want m-123", result.MachineID)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.StartedAt == nil || result.FinishedAt == nil {
		t.Error("expected StartedAt and FinishedAt to be set")
	}
}

func TestFlyRuntime_Run_429_ReturnsRetryable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsRetryable(err) {
		t.Errorf("expected retryable error, got %v", err)
	}
}

func TestFlyRuntime_Run_503_ReturnsRetryable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if !IsRetryable(err) {
		t.Errorf("expected retryable error, got %v", err)
	}
}

func TestFlyRuntime_Run_422_ReturnsFatal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":"invalid config"}`))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got %v", err)
	}
}

func TestFlyRuntime_Run_500_ReturnsRetryable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if !IsRetryable(err) {
		t.Errorf("expected retryable error, got %v", err)
	}
}

func TestFlyRuntime_Run_InvalidPreset_ReturnsFatal(t *testing.T) {
	t.Parallel()
	runtime := NewFlyRuntime("tok", "app")
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "invalid",
	})
	if !IsFatal(err) {
		t.Errorf("expected fatal error for invalid preset, got %v", err)
	}
}

func TestFlyRuntime_Stop(t *testing.T) {
	t.Parallel()
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stop") {
			called = true
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	err := runtime.Stop(context.Background(), "m-123")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !called {
		t.Error("stop endpoint not called")
	}
}

func TestFlyRuntime_Destroy(t *testing.T) {
	t.Parallel()
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			called = true
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	err := runtime.Destroy(context.Background(), "m-123")
	if err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !called {
		t.Error("destroy endpoint not called")
	}
}

func TestFlyRuntime_GetLogs(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("line1\nline2\n"))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	logs, err := runtime.GetLogs(context.Background(), "m-123", 100)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	if !strings.Contains(logs, "line1") {
		t.Errorf("logs = %q, expected line1", logs)
	}
}

func TestFlyRuntime_Run_CPUKind_PerformanceForLargePresets(t *testing.T) {
	t.Parallel()

	var receivedBody flyCreateRequest
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/apps/app/machines", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-1"})
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-1/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"events": []map[string]any{{"type": "exit", "exit_code": 0}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "medium-1x", // 2 CPUs, 4096 MB → performance
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if receivedBody.Config.Guest.CPUKind != "performance" {
		t.Errorf("CPUKind = %q, want performance for medium-1x", receivedBody.Config.Guest.CPUKind)
	}
}

func TestFlyRuntime_Run_CPUKind_SharedForSmallPresets(t *testing.T) {
	t.Parallel()

	var receivedBody flyCreateRequest
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/apps/app/machines", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-1"})
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-1/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"events": []map[string]any{{"type": "exit", "exit_code": 0}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro", // 1 CPU, 256 MB → shared
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if receivedBody.Config.Guest.CPUKind != "shared" {
		t.Errorf("CPUKind = %q, want shared for micro", receivedBody.Config.Guest.CPUKind)
	}
}

func TestFlyRuntime_Status_AllStates(t *testing.T) {
	t.Parallel()

	states := map[string]MachineStatus{
		"created":   MachineStatusCreated,
		"starting":  MachineStatusStarting,
		"started":   MachineStatusRunning,
		"running":   MachineStatusRunning,
		"stopping":  MachineStatusStopping,
		"stopped":   MachineStatusStopped,
		"destroyed": MachineStatusDestroyed,
		"weird":     MachineStatusUnknown,
	}
	for state, want := range states {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-1", State: state})
		}))

		runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
		got, err := runtime.Status(context.Background(), "m-1")
		srv.Close()
		if err != nil {
			t.Errorf("Status(%q) error = %v", state, err)
			continue
		}
		if got != want {
			t.Errorf("Status(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestFlyRuntime_Run_ConnectionRefused_ReturnsRetryable(t *testing.T) {
	t.Parallel()
	runtime := NewFlyRuntime("tok", "app").WithBaseURL("http://127.0.0.1:1") // Nothing listening.
	_, err := runtime.Run(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsRetryable(err) {
		t.Errorf("connection refused should be retryable, got: %v", err)
	}
}

func TestFlyRuntime_Create_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/machines") {
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-new-123", State: "started"})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	machineID, err := runtime.Create(context.Background(), RunRequest{
		ImageURI:      "registry.example.com/myapp:v1",
		MachinePreset: "small-1x",
		Region:        "iad",
		Env:           map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if machineID != "m-new-123" {
		t.Errorf("machineID = %q, want m-new-123", machineID)
	}
}

func TestFlyRuntime_Create_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Create(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if !IsRetryable(err) {
		t.Errorf("expected retryable error for unmarshal failure, got: %v", err)
	}
}

func TestFlyRuntime_Wait_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps/app/machines/m-1/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "m-1",
			"state": "stopped",
			"events": []map[string]any{
				{"type": "exit", "exit_code": 0},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	result, err := runtime.Wait(context.Background(), "m-1", 60)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.MachineID != "m-1" {
		t.Errorf("MachineID = %q, want m-1", result.MachineID)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.StartedAt == nil || result.FinishedAt == nil {
		t.Error("expected StartedAt and FinishedAt to be set")
	}
}

func TestFlyRuntime_Wait_NonZeroExit(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps/app/machines/m-oom/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-oom", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "m-oom",
			"state": "stopped",
			"events": []map[string]any{
				{"type": "exit", "exit_code": 137},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	result, err := runtime.Wait(context.Background(), "m-oom", 60)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.ExitCode != 137 {
		t.Errorf("ExitCode = %d, want 137 (OOM killed)", result.ExitCode)
	}
}

func TestFlyRuntime_Wait_NoExitEvent(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps/app/machines/m-noexit/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /v1/apps/app/machines/m-noexit", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "m-noexit",
			"state": "stopped",
			"events": []map[string]any{
				{"type": "start"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	result, err := runtime.Wait(context.Background(), "m-noexit", 60)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 (no exit event)", result.ExitCode)
	}
}

func TestFlyRuntime_Create_ErrorRedacted(t *testing.T) {
	t.Parallel()

	// Return a long error body that should be truncated.
	longBody := strings.Repeat("sensitive-data-", 50) // 750 chars
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	runtime := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
	_, err := runtime.Create(context.Background(), RunRequest{
		ImageURI:      "img:latest",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if len(errMsg) > 300 {
		t.Errorf("error message too long (%d chars), should be truncated", len(errMsg))
	}
	if !strings.Contains(errMsg, "truncated") {
		t.Error("expected truncated marker in error message")
	}
}
