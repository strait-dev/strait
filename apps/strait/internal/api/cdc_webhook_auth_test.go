package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/cdc"
	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.Equal(
		t, http.StatusUnauthorized,
		rec.
			Code)
	require.Contains(t,
		rec.
			Body.String(), "invalid or missing internal secret")

	req = httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(
		t, http.StatusUnauthorized,
		rec.
			Code)
	require.Contains(t,
		rec.
			Body.String(), "invalid signature")
}
