package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func validCloudflareMetadata() json.RawMessage {
	return MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider:          ProviderNameCloudflare,
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		DispatchWorkerURL: "https://dispatch.example.com",
		CompatibilityDate: "2026-03-29",
	})
}

func TestDispatchCloudflareRun_NilHTTPClient(t *testing.T) {
	t.Parallel()
	svc := &localService{dispatchHTTP: nil, internalSecret: "secret"}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ProviderMetadata: validCloudflareMetadata(),
	}, RuntimeDispatchEnvelope{})
	if err == nil {
		t.Fatal("expected error for nil HTTP client")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDispatchCloudflareRun_EmptyInternalSecret(t *testing.T) {
	t.Parallel()
	svc := &localService{
		dispatchHTTP:   &http.Client{},
		internalSecret: "",
	}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ProviderMetadata: validCloudflareMetadata(),
	}, RuntimeDispatchEnvelope{})
	if err == nil {
		t.Fatal("expected error for empty internal secret")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDispatchCloudflareRun_CorruptMetadata(t *testing.T) {
	t.Parallel()
	svc := &localService{
		dispatchHTTP:   &http.Client{},
		internalSecret: "secret",
	}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ProviderMetadata: json.RawMessage(`not-json`),
	}, RuntimeDispatchEnvelope{})
	if err == nil {
		t.Fatal("expected error for corrupt metadata")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDispatchCloudflareRun_Success(t *testing.T) {
	t.Parallel()

	var gotAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	metadata := MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider:          ProviderNameCloudflare,
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		DispatchWorkerURL: srv.URL,
		CompatibilityDate: "2026-03-29",
	})

	svc := &localService{
		dispatchHTTP:   srv.Client(),
		internalSecret: "test-secret",
	}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ID: "dep-1", Provider: ProviderNameCloudflare, ProviderMetadata: metadata,
	}, RuntimeDispatchEnvelope{Run: RuntimeDispatchRun{ID: "run-1"}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotAuth != "Bearer test-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
}

func TestDispatchCloudflareRun_ServerReturns500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	metadata := MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider: ProviderNameCloudflare, Namespace: "ns", ScriptName: "s",
		DispatchWorkerURL: srv.URL, CompatibilityDate: "2026-03-29",
	})

	svc := &localService{dispatchHTTP: srv.Client(), internalSecret: "secret"}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ProviderMetadata: metadata,
	}, RuntimeDispatchEnvelope{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDispatchCloudflareRun_ServerReturns201(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201 is 2xx, should be success.
	}))
	defer srv.Close()

	metadata := MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider: ProviderNameCloudflare, Namespace: "ns", ScriptName: "s",
		DispatchWorkerURL: srv.URL, CompatibilityDate: "2026-03-29",
	})

	svc := &localService{dispatchHTTP: srv.Client(), internalSecret: "secret"}
	err := svc.dispatchCloudflareRun(context.Background(), &domain.AgentDeployment{
		ProviderMetadata: metadata,
	}, RuntimeDispatchEnvelope{})
	if err != nil {
		t.Fatalf("201 should be success, got error = %v", err)
	}
}
