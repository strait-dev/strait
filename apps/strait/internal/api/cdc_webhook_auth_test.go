package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/cdc"
	"strait/internal/config"
	"strait/internal/domain"
)

func TestCDCWebhookRouteRequiresInternalSecretAndSequinSignature(t *testing.T) {
	cfg := &config.Config{
		InternalSecret: "test-secret-value",
		JWTSigningKey:  testJWTSigningKey,
	}
	receiver := cdc.NewWebhookReceiver(nil, nil, cdc.WithWebhookSecret("sequin-webhook-secret"))
	srv := NewServer(ServerDeps{
		Config:             cfg,
		Store:              &APIStoreMock{},
		Queue:              &mockQueue{},
		CDCWebhookReceiver: receiver,
		Edition:            domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing internal secret status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "invalid or missing internal secret") {
		t.Fatalf("missing internal secret body = %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing Sequin signature status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "invalid signature") {
		t.Fatalf("missing Sequin signature body = %q", rec.Body.String())
	}
}
