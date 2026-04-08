package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryPrometheusScalar(t *testing.T) {
	t.Run("returns scalar value from success response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/query" {
				t.Fatalf("path = %s, want /api/v1/query", r.URL.Path)
			}
			if got := r.URL.Query().Get("query"); got == "" {
				t.Fatal("expected query param to be set")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1712415000,"12.5"]}]}}`))
		}))
		defer srv.Close()

		value, err := queryPrometheusScalar(context.Background(), srv.URL, "sum(rate(test_metric[5m]))")
		if err != nil {
			t.Fatalf("queryPrometheusScalar() error = %v", err)
		}
		if value != 12.5 {
			t.Fatalf("value = %v, want 12.5", value)
		}
	})

	t.Run("returns zero when no series returned", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}))
		defer srv.Close()

		value, err := queryPrometheusScalar(context.Background(), srv.URL, "sum(rate(test_metric[5m]))")
		if err != nil {
			t.Fatalf("queryPrometheusScalar() error = %v", err)
		}
		if value != 0 {
			t.Fatalf("value = %v, want 0", value)
		}
	})

	t.Run("returns error for non-success payload", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"error","error":"bad_data"}`))
		}))
		defer srv.Close()

		_, err := queryPrometheusScalar(context.Background(), srv.URL, "sum(rate(test_metric[5m]))")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error on http status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer srv.Close()

		_, err := queryPrometheusScalar(context.Background(), srv.URL, "sum(rate(test_metric[5m]))")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestPromRangeLiteral(t *testing.T) {
	if got := promRangeLiteral(0); got != "300s" {
		t.Fatalf("promRangeLiteral(0) = %s, want 300s", got)
	}
	if got := promRangeLiteral(90 * time.Second); got != "90s" {
		t.Fatalf("promRangeLiteral(90s) = %s, want 90s", got)
	}
}
