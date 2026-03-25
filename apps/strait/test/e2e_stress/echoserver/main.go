package main

import (
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	totalRequests  atomic.Int64
	failedRequests atomic.Int64
	slowRequests   atomic.Int64
)

func main() {
	// Fast echo - returns immediately.
	http.HandleFunc("/fast-echo", func(w http.ResponseWriter, r *http.Request) {
		totalRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"timestamp": time.Now().UTC(),
			"job_id":    r.Header.Get("X-Strait-Job-ID"),
			"run_id":    r.Header.Get("X-Strait-Run-ID"),
		})
	})

	// Slow process - 1-3s delay.
	http.HandleFunc("/slow-process", func(w http.ResponseWriter, _ *http.Request) {
		totalRequests.Add(1)
		slowRequests.Add(1)
		time.Sleep(time.Duration(1000+mathrand.Intn(2000)) * time.Millisecond) //nolint:gosec // G404: test code
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "processed"})
	})

	// Flaky - 30% failure rate.
	http.HandleFunc("/flaky", func(w http.ResponseWriter, _ *http.Request) {
		totalRequests.Add(1)
		if mathrand.Float64() < 0.3 { //nolint:gosec // G404: test code
			failedRequests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "random failure"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// Timeout - takes too long.
	http.HandleFunc("/timeout", func(w http.ResponseWriter, _ *http.Request) {
		totalRequests.Add(1)
		time.Sleep(10 * time.Minute)
		w.WriteHeader(http.StatusOK)
	})

	// Always fail.
	http.HandleFunc("/always-fail", func(w http.ResponseWriter, _ *http.Request) {
		totalRequests.Add(1)
		failedRequests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "intentional failure"})
	})

	// Webhook receiver.
	http.HandleFunc("/webhook-receiver", func(w http.ResponseWriter, _ *http.Request) {
		totalRequests.Add(1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"received": true})
	})

	// Stats.
	http.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_requests":  totalRequests.Load(),
			"failed_requests": failedRequests.Load(),
			"slow_requests":   slowRequests.Load(),
		})
	})

	// Health.
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	log.Println("echo server starting on :9000")
	srv := &http.Server{Addr: ":9000", ReadHeaderTimeout: 10 * time.Second} //nolint:mnd // test server.
	log.Fatal(srv.ListenAndServe())
}
