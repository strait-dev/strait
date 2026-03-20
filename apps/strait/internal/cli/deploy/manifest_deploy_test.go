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
			if req.Runtime != "node" {
				t.Errorf("expected runtime=node, got %q", req.Runtime)
			}
			if req.ArtifactURI != "https://example.com/artifact.tgz" {
				t.Errorf("expected artifact_uri to be forwarded, got %q", req.ArtifactURI)
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
			var req client.FinalizeDeploymentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode finalize request: %v", err)
			}
			if req.ProjectID != "proj-1" || req.Environment != "production" {
				t.Errorf("unexpected finalize request: %+v", req)
			}
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
		ConfigPath:  configPath,
		ArtifactURI: "https://example.com/artifact.tgz",
		OutDir:      outDir,
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
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
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
		ConfigPath:  configPath,
		ArtifactURI: "https://example.com/artifact.tgz",
		OutDir:      filepath.Join(dir, "out"),
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
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
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
		ConfigPath:  configPath,
		ArtifactURI: "https://example.com/artifact.tgz",
		DryRun:      true,
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
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
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
		ArtifactURI: "https://example.com/artifact.tgz",
		OutDir:      filepath.Join(dir, "out"),
	})
	if receivedEnv != "production" {
		t.Fatalf("expected environment=production, got %q", receivedEnv)
	}
}

func TestDeployManifest_EnvironmentOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
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
		ArtifactURI: "https://example.com/artifact.tgz",
		OutDir:      filepath.Join(dir, "out"),
	})
	if receivedEnv != "staging" {
		t.Fatalf("expected environment=staging, got %q", receivedEnv)
	}
}

func TestDeployManifest_RequiresRuntime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cli, err := client.New("https://example.com", "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	err = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath:  configPath,
		ArtifactURI: "https://example.com/artifact.tgz",
	})
	if err == nil || err.Error() != "manifest deploy requires project.runtime in the config file" {
		t.Fatalf("expected missing runtime error, got %v", err)
	}
}

func TestDeployManifest_RequiresArtifactURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cli, err := client.New("https://example.com", "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	err = DeployManifest(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
	})
	if err == nil || err.Error() != "manifest deploy requires --artifact-uri" {
		t.Fatalf("expected missing artifact error, got %v", err)
	}
}

func TestCreateManifestDeployment_DryRunSkipsArtifactValidation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cli, err := client.New("https://example.com", "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, manifest, _, _, err := CreateManifestDeployment(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		DryRun:     true,
		// No ArtifactURI -- should be OK for dry-run
	})
	if err != nil {
		t.Fatalf("dry-run should not require artifact URI: %v", err)
	}
	if manifest == nil {
		t.Fatal("expected manifest to be returned")
	}
}

func TestCreateManifestDeployment_DryRunStillRequiresRuntime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cli, err := client.New("https://example.com", "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, _, err = CreateManifestDeployment(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		DryRun:     true,
	})
	if err == nil {
		t.Fatal("expected error for missing runtime")
	}
	if err.Error() != "manifest deploy requires project.runtime in the config file" {
		t.Fatalf("expected runtime error, got: %v", err)
	}
}

func TestCreateManifestDeployment_RealDeployRequiresArtifactURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cli, err := client.New("https://example.com", "test-key", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, _, err = CreateManifestDeployment(context.Background(), cli, ManifestDeployOptions{
		ConfigPath: configPath,
		DryRun:     false,
		// No ArtifactURI
	})
	if err == nil {
		t.Fatal("expected error for missing artifact URI on real deploy")
	}
	if err.Error() != "manifest deploy requires --artifact-uri" {
		t.Fatalf("expected artifact URI error, got: %v", err)
	}
}

func TestCreateManifestDeployment_RealDeployWithAllInputs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var createCalled atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/deployments" {
			createCalled.Add(1)
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

	deployment, manifest, _, _, err := CreateManifestDeployment(context.Background(), cli, ManifestDeployOptions{
		ConfigPath:  configPath,
		DryRun:      false,
		ArtifactURI: "https://example.com/artifact.tgz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment == nil {
		t.Fatal("expected deployment to be returned")
	}
	if manifest == nil {
		t.Fatal("expected manifest to be returned")
	}
	if createCalled.Load() != 1 {
		t.Errorf("expected 1 create call, got %d", createCalled.Load())
	}
}
