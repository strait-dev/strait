package deploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/cli/client"
)

func TestDeployManifest_CreateAndFinalize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node","jobs":[{"slug":"j1","name":"J1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var createCalled, finalizeCalled atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments":
			createCalled.Add(1)
			var req client.CreateDeploymentVersionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			if req.ProjectID != "proj-1" {
				t.Errorf("expected project_id=proj-1, got %q", req.ProjectID)
			}
			if req.Checksum == "" {
				t.Error("expected checksum to be set")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(client.DeploymentVersion{
				ID:        "dep-1",
				ProjectID: "proj-1",
				Status:    "pending",
				CreatedAt: time.Now(),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments/dep-1/finalize":
			finalizeCalled.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	err = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		OutDir:     outDir,
	})
	if err != nil {
		t.Fatalf("DeployManifest: %v", err)
	}
	if createCalled.Load() != 1 {
		t.Errorf("expected 1 create call, got %d", createCalled.Load())
	}
	if finalizeCalled.Load() != 1 {
		t.Errorf("expected 1 finalize call, got %d", finalizeCalled.Load())
	}

	// Check manifest file was written
	if _, err := os.Stat(filepath.Join(outDir, "manifest.json")); err != nil {
		t.Errorf("manifest.json not written: %v", err)
	}
}

func TestDeployManifest_CreateFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var finalizeCalled atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/deployments" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		case r.URL.Path == "/v1/deployments/dep-1/finalize":
			finalizeCalled.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	err = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		OutDir:     filepath.Join(dir, "out"),
	})
	if err == nil {
		t.Fatal("expected error when create fails")
	}
	if finalizeCalled.Load() != 0 {
		t.Error("finalize should not be called when create fails")
	}
}

func TestDeployManifest_DryRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var apiCalled atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalled.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	err = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("DeployManifest dry-run: %v", err)
	}
	if apiCalled.Load() != 0 {
		t.Error("no API calls should be made in dry-run mode")
	}
}

func TestDeployManifest_EnvironmentDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var receivedEnv string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/deployments" {
			var req client.CreateDeploymentVersionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			receivedEnv = req.Environment
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(client.DeploymentVersion{ID: "dep-1", CreatedAt: time.Now()})
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_ = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		OutDir:     filepath.Join(dir, "out"),
	})
	if receivedEnv != "production" {
		t.Fatalf("expected environment=production, got %q", receivedEnv)
	}
}

func TestDeployManifest_EnvironmentOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var receivedEnv string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/deployments" {
			var req client.CreateDeploymentVersionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			receivedEnv = req.Environment
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(client.DeploymentVersion{ID: "dep-1", CreatedAt: time.Now()})
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_ = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath:  configPath,
		Environment: "staging",
		OutDir:      filepath.Join(dir, "out"),
	})
	if receivedEnv != "staging" {
		t.Fatalf("expected environment=staging, got %q", receivedEnv)
	}
}
