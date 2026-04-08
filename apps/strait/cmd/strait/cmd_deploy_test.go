package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupDeployTestDir writes a minimal strait.json and a source file.
func setupDeployTestDir(t *testing.T, projectID, runtime string) string {
	t.Helper()
	dir := t.TempDir()
	sc := map[string]any{
		"project": map[string]string{"id": projectID},
		"deploy":  map[string]string{"runtime": runtime},
	}
	data, _ := json.Marshal(sc)
	if err := os.WriteFile(filepath.Join(dir, "strait.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDeployCommand_DryRun(t *testing.T) {
	dir := setupDeployTestDir(t, "proj_test", "python")

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(t.TempDir()) })

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	// --dry-run exits before network calls, so no --job needed.
	cmd := newDeployCommand()
	cmd.SetArgs([]string{"--dry-run"})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("deploy --dry-run: %v", err)
	}
}

func TestDeployCommand_UploadAndConfirm(t *testing.T) {
	uploadCalled := false
	confirmCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs":
			// Job slug lookup.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "job_123", "slug": "my-job", "source_type": "image",
						"enabled": true, "version": 1, "created_at": "2024-01-01T00:00:00Z",
						"updated_at": "2024-01-01T00:00:00Z"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments":
			// Create deployment.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_abc", "status": "pending", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
				"upload_url": "http://" + r.Host + "/fake-upload",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/fake-upload":
			uploadCalled = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments/dep_abc/confirm":
			confirmCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_abc", "status": "building", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_abc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_abc", "status": "ready", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_abc/logs":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"done\":true}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := setupDeployTestDir(t, "proj_test", "python")
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(t.TempDir()) })

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_API_URL", srv.URL)
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cmd := newDeployCommand()
	cmd.SetArgs([]string{"--job", "my-job"})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	if !uploadCalled {
		t.Error("expected upload PUT to be called")
	}
	if !confirmCalled {
		t.Error("expected confirm POST to be called")
	}
}

func TestDeployCommand_ExitsErrorOnFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "job_123", "slug": "my-job", "source_type": "image",
						"enabled": true, "version": 1, "created_at": "2024-01-01T00:00:00Z",
						"updated_at": "2024-01-01T00:00:00Z"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_fail", "status": "pending", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
				"upload_url": "http://" + r.Host + "/fake-upload",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/fake-upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments/dep_fail/confirm":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_fail", "status": "building", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_fail":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_fail", "status": "failed", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"error_message": "compilation error on line 42",
					"created_at":    "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_fail/logs":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"done\":true}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := setupDeployTestDir(t, "proj_test", "python")
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(t.TempDir()) })

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_API_URL", srv.URL)
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cmd := newDeployCommand()
	cmd.SetArgs([]string{"--job", "my-job"})
	err := cmd.ExecuteContext(t.Context())
	if err == nil {
		t.Fatal("expected error for failed deployment")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestDeployCommand_ExitsErrorOnTimedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "job_123", "slug": "my-job", "source_type": "image",
						"enabled": true, "version": 1, "created_at": "2024-01-01T00:00:00Z",
						"updated_at": "2024-01-01T00:00:00Z"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_to", "status": "pending", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
				"upload_url": "http://" + r.Host + "/fake-upload",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/fake-upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job_123/deployments/dep_to/confirm":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_to", "status": "building", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_to":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{
					"id": "dep_to", "status": "timed_out", "version": 1,
					"runtime": "python", "source_hash": "abc", "source_size_bytes": 100,
					"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job_123/deployments/dep_to/logs":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"done\":true}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := setupDeployTestDir(t, "proj_test", "python")
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(t.TempDir()) })

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_API_URL", srv.URL)
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cmd := newDeployCommand()
	cmd.SetArgs([]string{"--job", "my-job"})
	err := cmd.ExecuteContext(t.Context())
	if err == nil {
		t.Fatal("expected error for timed_out deployment")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message for timed_out")
	}
}

func TestDeployCommand_NoJobFlagErrors(t *testing.T) {
	dir := setupDeployTestDir(t, "proj_test", "python")
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(t.TempDir()) })

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cmd := newDeployCommand()
	cmd.SetArgs([]string{})
	if err := cmd.ExecuteContext(t.Context()); err == nil {
		t.Fatal("expected error when --job is missing")
	}
}
