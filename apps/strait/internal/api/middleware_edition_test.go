package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireCloudEdition_Community(t *testing.T) {
	s := &Server{edition: domain.EditionCommunity}

	handler := s.requireCloudEdition(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.
		StatusPaymentRequired,
		rr.Code,
	)

	var body map[string]string
	require.NoError(t,
		json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "this feature requires Strait Cloud",

		body["error"])
	assert.Equal(t, "community",
		body["edition"])
	assert.Equal(t, "https://strait.dev/pricing",

		body["upgrade"])

}

func TestRequireCloudEdition_Cloud(t *testing.T) {
	s := &Server{edition: domain.EditionCloud}

	called := false
	handler := s.requireCloudEdition(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.
		StatusOK,
		rr.Code)
	require.True(t, called)

}
