package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestBuildCloudflareScriptName(t *testing.T) {
	t.Parallel()

	got := buildCloudflareScriptName("019d-353d-1e74-70de-8ff9-cfb46a4b8927", 12)
	want := "agent-019d353d1e7470de8ff9cfb46a4b8927-v12"
	if got != want {
		t.Fatalf("buildCloudflareScriptName() = %q, want %q", got, want)
	}
}

func TestCloudflareAPIClientUpsertScriptBuildsMultipartRequest(t *testing.T) {
	t.Parallel()

	var seenAuth string
	var seenPath string
	var seenMetadata map[string]any
	var seenSource string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("Content-Type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}
			payload, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll(part) error = %v", err)
			}
			switch part.FormName() {
			case "metadata":
				if err := json.Unmarshal(payload, &seenMetadata); err != nil {
					t.Fatalf("Unmarshal(metadata) error = %v", err)
				}
			case "worker.mjs":
				seenSource = string(payload)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"result":{"id":"agent-script","etag":"etag-123","compatibility_date":"2026-03-29"}}`)
	}))
	defer server.Close()

	client := NewCloudflareAPIClient(CloudflareConfig{
		AccountID: "acct-1",
		APIToken:  "token-1",
	}, WithCloudflareAPIBaseURL(server.URL))

	result, err := client.UpsertScript(context.Background(), CloudflareScriptUploadRequest{
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		CompatibilityDate: "2026-03-29",
		SandboxPolicy: CloudflareSandboxPolicy{
			Mode:          CloudflareSandboxModeDynamicWorker,
			DefaultAction: CloudflareSandboxDefaultActionDeny,
			AllowHosts:    []string{"api.openai.com"},
			NetworkClass:  "sandbox",
			PolicyTag:     "llm",
		},
		Tags:   []string{"strait-agent"},
		Source: `export default { async fetch() { return new Response("ok"); } };`,
	})
	if err != nil {
		t.Fatalf("UpsertScript() error = %v", err)
	}

	if seenAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q, want Bearer token-1", seenAuth)
	}
	if seenPath != "/accounts/acct-1/workers/dispatch/namespaces/ns-prod/scripts/agent-script" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenMetadata["main_module"] != "worker.mjs" {
		t.Fatalf("metadata.main_module = %v", seenMetadata["main_module"])
	}
	if seenMetadata["compatibility_date"] != "2026-03-29" {
		t.Fatalf("metadata.compatibility_date = %v", seenMetadata["compatibility_date"])
	}
	annotations, ok := seenMetadata["annotations"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.annotations = %T, want map[string]any", seenMetadata["annotations"])
	}
	if annotations["strait_sandbox_default_action"] != "deny" {
		t.Fatalf("annotations.strait_sandbox_default_action = %v", annotations["strait_sandbox_default_action"])
	}
	if annotations["strait_sandbox_allow_hosts"] != "api.openai.com" {
		t.Fatalf("annotations.strait_sandbox_allow_hosts = %v", annotations["strait_sandbox_allow_hosts"])
	}
	if !strings.Contains(seenSource, `return new Response("ok")`) {
		t.Fatalf("source = %q", seenSource)
	}
	if result.ETag != "etag-123" {
		t.Fatalf("result.ETag = %q, want etag-123", result.ETag)
	}
	if result.ContentSHA256 == "" {
		t.Fatal("expected content hash")
	}
}

func TestCloudflareProviderDeployReturnsMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"result":{"id":"agent-script","etag":"etag-123","compatibility_date":"2026-03-29"}}`)
	}))
	defer server.Close()

	provider := NewCloudflareProvider(CloudflareConfig{
		AccountID:         "acct-1",
		APIToken:          "token-1",
		DispatchNamespace: "ns-prod",
		DispatchWorkerURL: "https://dispatch.example.com",
		CompatibilityDate: "2026-03-29",
		SandboxMode:       CloudflareSandboxModeDynamicWorker,
	}, WithCloudflareAPIBaseURL(server.URL))

	raw, err := provider.Deploy(context.Background(), &domain.Agent{ID: "agent-1"}, &domain.AgentDeployment{
		ID:      "dep-1",
		Version: 2,
	})
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	metadata, err := ParseCloudflareDeploymentMetadata(raw)
	if err != nil {
		t.Fatalf("ParseCloudflareDeploymentMetadata() error = %v", err)
	}
	if metadata.ScriptName != "agent-agent1-v2" {
		t.Fatalf("metadata.ScriptName = %q", metadata.ScriptName)
	}
	if metadata.Namespace != "ns-prod" {
		t.Fatalf("metadata.Namespace = %q", metadata.Namespace)
	}
	if metadata.Etag != "etag-123" {
		t.Fatalf("metadata.Etag = %q, want etag-123", metadata.Etag)
	}
	if metadata.SandboxPolicy.Mode != CloudflareSandboxModeDynamicWorker {
		t.Fatalf("metadata.SandboxPolicy.Mode = %q", metadata.SandboxPolicy.Mode)
	}
	if metadata.SandboxPolicy.DefaultAction != CloudflareSandboxDefaultActionDeny {
		t.Fatalf("metadata.SandboxPolicy.DefaultAction = %q, want deny", metadata.SandboxPolicy.DefaultAction)
	}
	if metadata.SandboxPolicy.NetworkClass != "sandbox" {
		t.Fatalf("metadata.SandboxPolicy.NetworkClass = %q, want sandbox", metadata.SandboxPolicy.NetworkClass)
	}
	if metadata.SandboxPolicy.PolicyTag != "default" {
		t.Fatalf("metadata.SandboxPolicy.PolicyTag = %q, want default", metadata.SandboxPolicy.PolicyTag)
	}
}

func TestCloudflareProviderUndeployDeletesScript(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := NewCloudflareProvider(CloudflareConfig{
		AccountID:         "acct-1",
		APIToken:          "token-1",
		DispatchNamespace: "ns-prod",
		DispatchWorkerURL: "https://dispatch.example.com",
		CompatibilityDate: "2026-03-29",
		SandboxMode:       CloudflareSandboxModeDisabled,
	}, WithCloudflareAPIBaseURL(server.URL))

	err := provider.Undeploy(context.Background(), &domain.Agent{ID: "agent-1"}, &domain.AgentDeployment{
		ID:       "dep-1",
		Provider: ProviderNameCloudflare,
		ProviderMetadata: MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
			Provider:          ProviderNameCloudflare,
			Namespace:         "ns-prod",
			ScriptName:        "agent-agent1-v2",
			DispatchWorkerURL: "https://dispatch.example.com",
			CompatibilityDate: "2026-03-29",
		}),
	})
	if err != nil {
		t.Fatalf("Undeploy() error = %v", err)
	}

	if gotPath != "/accounts/acct-1/workers/dispatch/namespaces/ns-prod/scripts/agent-agent1-v2" {
		t.Fatalf("delete path = %q", gotPath)
	}
}

func TestBuildCloudflareMultipartUploadIncludesMetadataAndSource(t *testing.T) {
	t.Parallel()

	body, contentType, _, err := buildCloudflareMultipartUpload(CloudflareScriptUploadRequest{
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		CompatibilityDate: "2026-03-29",
		Tags:              []string{"strait-agent"},
		Bindings:          []map[string]any{{"name": "ENVIRONMENT", "type": "plain_text", "text": "prod"}},
		SandboxPolicy: CloudflareSandboxPolicy{
			Mode:          CloudflareSandboxModeDynamicWorker,
			DefaultAction: CloudflareSandboxDefaultActionDeny,
		},
		Source: `export default { async fetch() { return new Response("ok"); } };`,
	})
	if err != nil {
		t.Fatalf("buildCloudflareMultipartUpload() error = %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("mediaType = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])

	parts := map[string]string{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		payload, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("ReadAll(part) error = %v", err)
		}
		parts[part.FormName()] = string(payload)
	}

	if !strings.Contains(parts["metadata"], `"compatibility_date":"2026-03-29"`) {
		t.Fatalf("metadata part = %q", parts["metadata"])
	}
	if !strings.Contains(parts["metadata"], `"strait_sandbox_mode":"dynamic_worker"`) {
		t.Fatalf("metadata part = %q", parts["metadata"])
	}
	if !strings.Contains(parts["worker.mjs"], `return new Response("ok")`) {
		t.Fatalf("worker part = %q", parts["worker.mjs"])
	}
}
