package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestDispatch_IncludesRunTokenIssuer(t *testing.T) {
	t.Parallel()

	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("X-Run-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := newTestExecutor(t, &mockExecutorStore{}, &mockExecQueue{}, time.Hour, srv.Client())
	exec.jwtSigningKey = "0123456789abcdef0123456789abcdef"

	if err := exec.dispatch(context.Background(), testJob(srv.URL, 1, 5), testRun(1)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	assertRunTokenIssuer(t, token, exec.jwtSigningKey)
}

func TestTracedDispatch_IncludesRunTokenIssuer(t *testing.T) {
	t.Parallel()

	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("X-Run-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := newTestExecutor(t, &mockExecutorStore{}, &mockExecQueue{}, time.Hour, srv.Client())
	exec.jwtSigningKey = "abcdef0123456789abcdef0123456789"

	if _, _, err := exec.tracedDispatch(context.Background(), testJob(srv.URL, 1, 5), testRun(1)); err != nil {
		t.Fatalf("tracedDispatch: %v", err)
	}

	assertRunTokenIssuer(t, token, exec.jwtSigningKey)
}

func assertRunTokenIssuer(t *testing.T, token, signingKey string) {
	t.Helper()
	if token == "" {
		t.Fatal("expected X-Run-Token header")
	}

	claims := struct {
		Attempt int `json:"attempt,omitempty"`
		jwt.RegisteredClaims
	}{}
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(signingKey), nil
	})
	if err != nil {
		t.Fatalf("parse run token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("parsed run token invalid")
	}
	if claims.Issuer != "strait:run-token" {
		t.Fatalf("claims.Issuer = %q, want %q", claims.Issuer, "strait:run-token")
	}
	if claims.Attempt != 1 {
		t.Fatalf("claims.Attempt = %d, want 1", claims.Attempt)
	}
}
