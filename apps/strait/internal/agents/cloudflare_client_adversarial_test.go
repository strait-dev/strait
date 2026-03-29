package agents

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestCloudflareAPIClientUpsertScriptRejectsMalformedSuccessBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"result":`)
	}))
	defer server.Close()

	client := NewCloudflareAPIClient(CloudflareConfig{
		AccountID: "acct-1",
		APIToken:  "token-1",
	}, WithCloudflareAPIBaseURL(server.URL))

	_, err := client.UpsertScript(context.Background(), CloudflareScriptUploadRequest{
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		CompatibilityDate: "2026-03-29",
		Source:            `export default { async fetch() { return new Response("ok"); } };`,
	})
	if err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestCloudflareAPIClientUpsertScriptHandlesCloudflareErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":1001,"message":"script already exists"}]}`)
	}))
	defer server.Close()

	client := NewCloudflareAPIClient(CloudflareConfig{
		AccountID: "acct-1",
		APIToken:  "token-1",
	}, WithCloudflareAPIBaseURL(server.URL))

	_, err := client.UpsertScript(context.Background(), CloudflareScriptUploadRequest{
		Namespace:         "ns-prod",
		ScriptName:        "agent-script",
		CompatibilityDate: "2026-03-29",
		Source:            `export default { async fetch() { return new Response("ok"); } };`,
	})
	if err == nil {
		t.Fatal("expected cloudflare api error")
	}

	apiErr := &CloudflareAPIError{}
	if !strings.Contains(err.Error(), "script already exists") {
		t.Fatalf("error = %v", err)
	}
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected CloudflareAPIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("apiErr.StatusCode = %d, want %d", apiErr.StatusCode, http.StatusConflict)
	}
}

func TestCloudflareProviderUndeployRejectsCorruptMetadata(t *testing.T) {
	t.Parallel()

	provider := NewCloudflareProvider(CloudflareConfig{
		AccountID:         "acct-1",
		APIToken:          "token-1",
		DispatchNamespace: "ns-prod",
		DispatchWorkerURL: "https://dispatch.example.com",
		CompatibilityDate: "2026-03-29",
	})

	err := provider.Undeploy(context.Background(), nil, &domain.AgentDeployment{
		Provider:         ProviderNameCloudflare,
		ProviderMetadata: []byte(`{"provider":"cloudflare"}`),
	})
	if err == nil {
		t.Fatal("expected corrupt metadata error")
	}
}
