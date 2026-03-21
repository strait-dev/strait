package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestRequireCloudEdition_Community(t *testing.T) {
	s := &Server{edition: domain.EditionCommunity}

	handler := s.requireCloudEdition(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "this feature requires Strait Cloud" {
		t.Errorf("unexpected error message: %s", body["error"])
	}
	if body["edition"] != "community" {
		t.Errorf("unexpected edition: %s", body["edition"])
	}
	if body["upgrade"] != "https://strait.dev/pricing" {
		t.Errorf("unexpected upgrade URL: %s", body["upgrade"])
	}
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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !called {
		t.Fatal("next handler was not called for cloud edition")
	}
}
