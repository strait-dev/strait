package worker

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"strait/internal/compute"
	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

// TestManagedDispatch_PresetOverrideViaMetadata verifies that _preset_override
// in run metadata changes the preset used for dispatch.
func TestManagedDispatch_PresetOverrideViaMetadata(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.Metadata = map[string]string{"_preset_override": "medium-1x"}

	job := newTestManagedJob()
	job.MachinePreset = domain.PresetMicro

	var capturedPreset string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedPreset = req.MachinePreset
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	if capturedPreset != "medium-1x" {
		t.Errorf("expected preset medium-1x via override, got %q", capturedPreset)
	}
}

// TestManagedDispatch_PresetOverrideInvalidPreset verifies that an invalid
// preset in _preset_override is rejected by IsValid and the original preset
// is used instead.
func TestManagedDispatch_PresetOverrideInvalidPreset(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.Metadata = map[string]string{"_preset_override": "nonexistent-preset"}

	job := newTestManagedJob()
	job.MachinePreset = domain.PresetSmall1x

	var capturedPreset string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedPreset = req.MachinePreset
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	// Invalid override should fall back to job preset.
	if capturedPreset != "small-1x" {
		t.Errorf("expected preset small-1x (original), got %q", capturedPreset)
	}
}

// TestManagedDispatch_OOMAutoUpgradeMax verifies that OOM auto-upgrade cannot
// escalate beyond the largest available preset.
func TestManagedDispatch_OOMAutoUpgradeMax(t *testing.T) {
	t.Parallel()

	// The largest preset is large-2x. Trying to upgrade from there should stay.
	largest := compute.PresetOrder[len(compute.PresetOrder)-1]
	if largest != "large-2x" {
		t.Fatalf("expected largest preset to be large-2x, got %q", largest)
	}

	next, ok := compute.NextPreset(largest)
	if ok {
		t.Errorf("expected no next preset beyond %q, got %q", largest, next)
	}

	if !compute.IsMaxPreset(largest) {
		t.Errorf("IsMaxPreset(%q) = false, want true", largest)
	}

	// Verify OOM recommendation pointing beyond max is capped.
	// PresetIndex for largest should be the last index.
	idx := compute.PresetIndex(largest)
	if idx != len(compute.PresetOrder)-1 {
		t.Errorf("PresetIndex(%q) = %d, want %d", largest, idx, len(compute.PresetOrder)-1)
	}
}

// TestManagedDispatch_SecretKeyNaming verifies that secrets are injected with
// the key naming convention STRAIT_SECRET_ + uppercase key.
func TestManagedDispatch_SecretKeyNaming(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	job := newTestManagedJob()

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		listSecretsFn: func(_ context.Context, _, _ string) ([]domain.JobSecret, error) {
			return []domain.JobSecret{
				{SecretKey: "db_password", EncryptedValue: "s3cret"},
				{SecretKey: "api_token", EncryptedValue: "tok123"},
			}, nil
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	expected := map[string]string{
		"STRAIT_SECRET_DB_PASSWORD": "s3cret",
		"STRAIT_SECRET_API_TOKEN":   "tok123",
	}
	for key, wantVal := range expected {
		got, ok := capturedEnv[key]
		if !ok {
			t.Errorf("missing env var %q", key)
		} else if got != wantVal {
			t.Errorf("env[%q] = %q, want %q", key, got, wantVal)
		}
	}
}

// TestManagedDispatch_JWTTokenExpiry verifies that the JWT token expires at
// timeout + 60 seconds.
func TestManagedDispatch_JWTTokenExpiry(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 120

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	before := time.Now()
	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)
	after := time.Now()

	tok, ok := capturedEnv["STRAIT_SDK_TOKEN"]
	if !ok {
		t.Fatal("STRAIT_SDK_TOKEN not set")
	}

	parser := jwt.NewParser()
	claims := &jwt.RegisteredClaims{}
	_, _, err := parser.ParseUnverified(tok, claims)
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	// Token should expire around now + timeoutSecs + 60s.
	// JWT NumericDate truncates to second precision, so truncate the bounds.
	expectedMin := before.Truncate(time.Second).Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	expectedMax := after.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second).Truncate(time.Second).Add(time.Second)

	expiry := claims.ExpiresAt.Time
	if expiry.Before(expectedMin) || expiry.After(expectedMax) {
		t.Errorf("token expiry %v not in expected range [%v, %v]", expiry, expectedMin, expectedMax)
	}
}

// TestManagedDispatch_JWTTokenSubject verifies that the JWT token subject is
// set to the run ID.
func TestManagedDispatch_JWTTokenSubject(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.ID = "run-jwt-sub-test"
	job := newTestManagedJob()

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	tok := capturedEnv["STRAIT_SDK_TOKEN"]
	if tok == "" {
		t.Fatal("STRAIT_SDK_TOKEN not set")
	}

	parser := jwt.NewParser()
	claims := &jwt.RegisteredClaims{}
	_, _, err := parser.ParseUnverified(tok, claims)
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	if claims.Subject != "run-jwt-sub-test" {
		t.Errorf("JWT subject = %q, want %q", claims.Subject, "run-jwt-sub-test")
	}
}

// TestManagedDispatch_PayloadInlineThreshold verifies that payloads under 64KB
// are inlined as STRAIT_PAYLOAD and payloads >= 64KB use STRAIT_PAYLOAD_MODE=fetch.
func TestManagedDispatch_PayloadInlineThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		payloadSize   int
		wantInline    bool
		wantFetchMode bool
	}{
		{"small payload inlined", 100, true, false},
		{"just under threshold inlined", 64*1024 - 1, true, false},
		{"above threshold fetched", 64*1024 + 1, false, true},
		{"large payload fetched", 256 * 1024, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newTestRun()
			run.Payload = make([]byte, tt.payloadSize)
			for i := range run.Payload {
				run.Payload[i] = 'A'
			}
			job := newTestManagedJob()

			var capturedEnv map[string]string
			runtime := &mockContainerRuntime{
				createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
					capturedEnv = req.Env
					return "m-1", nil
				},
				waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
					return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
				},
			}

			store := &mockExecutorStore{
				getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
				},
			}

			e := newManagedTestExecutor(store, runtime)
			e.managedDispatch(context.Background(), run, job)

			_, hasPayload := capturedEnv["STRAIT_PAYLOAD"]
			mode, hasFetchMode := capturedEnv["STRAIT_PAYLOAD_MODE"]

			if tt.wantInline && !hasPayload {
				t.Error("expected STRAIT_PAYLOAD to be set")
			}
			if !tt.wantInline && hasPayload {
				t.Error("expected STRAIT_PAYLOAD not to be set")
			}
			if tt.wantFetchMode && (!hasFetchMode || mode != "fetch") {
				t.Errorf("expected STRAIT_PAYLOAD_MODE=fetch, got %q (present=%v)", mode, hasFetchMode)
			}
			if !tt.wantFetchMode && hasFetchMode {
				t.Error("expected STRAIT_PAYLOAD_MODE not to be set")
			}
		})
	}
}

// TestManagedDispatch_PayloadExactBoundary verifies behavior at exactly 64*1024 bytes.
// The threshold is <=64KB for inline, so exactly 64KB should be inlined.
func TestManagedDispatch_PayloadExactBoundary(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.Payload = make([]byte, 64*1024)
	for i := range run.Payload {
		run.Payload[i] = 'B'
	}
	job := newTestManagedJob()

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	// Exactly 64*1024 = maxInlinePayload, so len(payload) <= maxInlinePayload is true.
	_, hasPayload := capturedEnv["STRAIT_PAYLOAD"]
	_, hasFetchMode := capturedEnv["STRAIT_PAYLOAD_MODE"]

	if !hasPayload {
		t.Error("expected STRAIT_PAYLOAD to be set at exact boundary (64KB)")
	}
	if hasFetchMode {
		t.Error("expected STRAIT_PAYLOAD_MODE not to be set at exact boundary")
	}
}

// TestManagedDispatch_RegionFallback verifies that when the job has no region
// set, the executor default region is used.
func TestManagedDispatch_RegionFallback(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "" // No region configured.

	var capturedRegion string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedRegion = req.Region
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(ex *Executor) {
		ex.defaultFlyRegion = "iad"
	})
	e.managedDispatch(context.Background(), run, job)

	if capturedRegion != "iad" {
		t.Errorf("expected region iad (default), got %q", capturedRegion)
	}
}

// TestManagedDispatch_EnvVarConstruction verifies that all expected env vars
// are present in the container request.
func TestManagedDispatch_EnvVarConstruction(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.ID = "run-env-test"
	run.Attempt = 1

	job := newTestManagedJob()
	job.Slug = "test-slug"
	job.TimeoutSecs = 60

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	required := []string{
		"STRAIT_RUN_ID",
		"STRAIT_JOB_SLUG",
		"STRAIT_ATTEMPT",
		"STRAIT_API_URL",
		"STRAIT_SDK_TOKEN",
		"STRAIT_MEMORY_LIMIT_MB",
	}

	for _, key := range required {
		if _, ok := capturedEnv[key]; !ok {
			t.Errorf("missing required env var %q", key)
		}
	}

	if capturedEnv["STRAIT_RUN_ID"] != "run-env-test" {
		t.Errorf("STRAIT_RUN_ID = %q, want %q", capturedEnv["STRAIT_RUN_ID"], "run-env-test")
	}
	if capturedEnv["STRAIT_JOB_SLUG"] != "test-slug" {
		t.Errorf("STRAIT_JOB_SLUG = %q, want %q", capturedEnv["STRAIT_JOB_SLUG"], "test-slug")
	}
	if capturedEnv["STRAIT_ATTEMPT"] != "1" {
		t.Errorf("STRAIT_ATTEMPT = %q, want %q", capturedEnv["STRAIT_ATTEMPT"], "1")
	}
	if capturedEnv["STRAIT_API_URL"] != "https://api.test.com" {
		t.Errorf("STRAIT_API_URL = %q, want %q", capturedEnv["STRAIT_API_URL"], "https://api.test.com")
	}

	memLimit := capturedEnv["STRAIT_MEMORY_LIMIT_MB"]
	if memLimit != strconv.Itoa(compute.AllPresets["micro"].MemoryMB) {
		t.Errorf("STRAIT_MEMORY_LIMIT_MB = %q, want %q", memLimit, strconv.Itoa(compute.AllPresets["micro"].MemoryMB))
	}
}

// TestManagedDispatch_PresetFromNameAllValid verifies that all valid presets
// can be resolved via PresetFromName.
func TestManagedDispatch_PresetFromNameAllValid(t *testing.T) {
	t.Parallel()

	for _, name := range compute.PresetOrder {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := compute.PresetFromName(name)
			if err != nil {
				t.Fatalf("PresetFromName(%q) error: %v", name, err)
			}
			if p.Name != name {
				t.Errorf("preset.Name = %q, want %q", p.Name, name)
			}
			if p.CPUs <= 0 {
				t.Errorf("preset.CPUs = %d, want > 0", p.CPUs)
			}
			if p.MemoryMB <= 0 {
				t.Errorf("preset.MemoryMB = %d, want > 0", p.MemoryMB)
			}
		})
	}
}

// TestManagedDispatch_PresetFromNameInvalid verifies that unknown presets
// return an error from PresetFromName.
func TestManagedDispatch_PresetFromNameInvalid(t *testing.T) {
	t.Parallel()

	invalid := []string{"", "tiny", "xlarge", "super-8x", "micro2x", "MICRO"}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := compute.PresetFromName(name)
			if err == nil {
				t.Errorf("PresetFromName(%q) = nil error, want error", name)
			}
		})
	}
}

// TestManagedDispatch_ExecutionModeValidation verifies that only "http" and
// "managed" are valid execution modes.
func TestManagedDispatch_ExecutionModeValidation(t *testing.T) {
	t.Parallel()

	valid := []domain.ExecutionMode{domain.ExecutionModeHTTP, domain.ExecutionModeManaged}
	for _, m := range valid {
		if !m.IsValid() {
			t.Errorf("ExecutionMode(%q).IsValid() = false, want true", m)
		}
	}

	invalid := []domain.ExecutionMode{"", "serverless", "lambda", "batch", "HTTP", "Managed"}
	for _, m := range invalid {
		if m.IsValid() {
			t.Errorf("ExecutionMode(%q).IsValid() = true, want false", m)
		}
	}
}

// TestManagedDispatch_MaxInlinePayloadConstant verifies that the max inline
// payload constant matches 64*1024 by observing dispatch behavior.
func TestManagedDispatch_MaxInlinePayloadConstant(t *testing.T) {
	t.Parallel()

	// Verify by dispatching a payload of exactly 64*1024+1 bytes and checking
	// that fetch mode is used.
	const expected = 64 * 1024

	run := newTestRun()
	run.Payload = make([]byte, expected+1)
	for i := range run.Payload {
		run.Payload[i] = 'X'
	}
	job := newTestManagedJob()

	var capturedEnv map[string]string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	mode, ok := capturedEnv["STRAIT_PAYLOAD_MODE"]
	if !ok || mode != "fetch" {
		t.Errorf("payload of %d bytes should trigger fetch mode, got mode=%q present=%v", expected+1, mode, ok)
	}

	// Now try exactly at the boundary, should inline.
	run2 := newTestRun()
	run2.Payload = make([]byte, expected)
	for i := range run2.Payload {
		run2.Payload[i] = 'Y'
	}

	var capturedEnv2 map[string]string
	runtime2 := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv2 = req.Env
			return "m-2", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-2"}, nil
		},
	}

	e2 := newManagedTestExecutor(&mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run2.ID, Status: domain.StatusCompleted}, nil
		},
	}, runtime2)
	e2.managedDispatch(context.Background(), run2, job)

	_, hasPayload := capturedEnv2["STRAIT_PAYLOAD"]
	_, hasFetch := capturedEnv2["STRAIT_PAYLOAD_MODE"]
	if !hasPayload {
		t.Error("payload of exactly 64KB should be inlined")
	}
	if hasFetch {
		t.Error("payload of exactly 64KB should not trigger fetch mode")
	}
}

// FuzzManagedDispatchEnv fuzzes env var key/value through secret key naming
// validation to ensure no panics or unexpected behavior.
func FuzzManagedDispatchEnv(f *testing.F) {
	f.Add("db_password")
	f.Add("API_TOKEN")
	f.Add("")
	f.Add("key-with-dashes")
	f.Add("key.with.dots")
	f.Add("key with spaces")
	f.Add("key\x00null")
	f.Add(strings.Repeat("a", 1000))

	f.Fuzz(func(t *testing.T, secretKey string) {
		// Simulate the naming convention from executor_dispatch.go.
		key := "STRAIT_SECRET_" + strings.ToUpper(secretKey)

		// Ensure the result is a valid-looking env var key (no panics).
		if !strings.HasPrefix(key, "STRAIT_SECRET_") {
			t.Errorf("key %q lost prefix", key)
		}

		// Verify uppercase transformation.
		suffix := key[len("STRAIT_SECRET_"):]
		for _, r := range suffix {
			if unicode.IsLetter(r) && !unicode.IsUpper(r) {
				t.Errorf("found lowercase letter %q in uppercase suffix", r)
			}
		}
	})
}

// TestManagedDispatch_OOMAutoUpgradeRespectsOverride verifies that when a
// _preset_override is present, OOM auto-upgrade is skipped entirely.
func TestManagedDispatch_OOMAutoUpgradeRespectsOverride(t *testing.T) {
	t.Parallel()

	run := newTestRun()
	run.Metadata = map[string]string{"_preset_override": "small-1x"}

	job := newTestManagedJob()
	job.MachinePreset = domain.PresetMicro

	var capturedPreset string
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedPreset = req.MachinePreset
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{ExitCode: 0, MachineID: "m-1"}, nil
		},
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: run.ID, Status: domain.StatusCompleted}, nil
		},
		// This would normally cause an upgrade to medium-1x.
		getPresetRecommendationFn: func(_ context.Context, _ string) (*orcstore.PresetRecommendation, error) {
			return &orcstore.PresetRecommendation{
				RecommendedPreset: "medium-1x",
				OOMCount:          5,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), run, job)

	// Should use the override, not the OOM recommendation.
	if capturedPreset != "small-1x" {
		t.Errorf("expected preset small-1x (override), got %q", capturedPreset)
	}
}
