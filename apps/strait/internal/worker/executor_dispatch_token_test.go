package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
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
	require.NoError(
		t, exec.dispatch(context.Background(), testJob(srv.URL,

			1, 5), testRun(1)))

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
		require.Failf(t, "test failure",

			"tracedDispatch: %v", err)
	}

	assertRunTokenIssuer(t, token, exec.jwtSigningKey)
}

func assertRunTokenIssuer(t *testing.T, token, signingKey string) {
	t.Helper()
	require.NotEqual(t, "", token)

	claims := struct {
		Attempt int `json:"attempt,omitempty"`
		jwt.RegisteredClaims
	}{}
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(signingKey), nil
	})
	require.NoError(
		t, err)
	require.True(t,
		parsed.Valid,
	)
	require.Equal(t,
		"strait:run-token",
		claims.Issuer,
	)
	require.EqualValues(t, 1, claims.Attempt)

}
