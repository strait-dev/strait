//go:build loadtest

package loadtest

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// TestServer provides HTTP endpoints that simulate real job targets.
// It runs on localhost during load tests and serves as the endpoint_url
// for HTTP-mode jobs.
type TestServer struct {
	srv     *http.Server
	addr    string
	stats   ServerStats
	started time.Time
}

// ServerStats tracks request counts across all endpoints.
type ServerStats struct {
	FastEcho     atomic.Int64
	SlowProcess  atomic.Int64
	VariableLoad atomic.Int64
	Flaky        atomic.Int64
	MemoryHeavy  atomic.Int64
	CostReporter atomic.Int64
	Total        atomic.Int64
}

// ServerStatsSnapshot is a point-in-time snapshot of server stats.
type ServerStatsSnapshot struct {
	FastEcho     int64 `json:"fast_echo"`
	SlowProcess  int64 `json:"slow_process"`
	VariableLoad int64 `json:"variable_load"`
	Flaky        int64 `json:"flaky"`
	MemoryHeavy  int64 `json:"memory_heavy"`
	CostReporter int64 `json:"cost_reporter"`
	Total        int64 `json:"total"`
}

// NewTestServer creates a test HTTP server on the given port.
func NewTestServer(port int) *TestServer {
	ts := &TestServer{
		addr:    fmt.Sprintf(":%d", port),
		started: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /fast-echo", ts.handleFastEcho)
	mux.HandleFunc("POST /slow-process", ts.handleSlowProcess)
	mux.HandleFunc("POST /variable-load", ts.handleVariableLoad)
	mux.HandleFunc("POST /flaky", ts.handleFlaky)
	mux.HandleFunc("POST /memory-heavy", ts.handleMemoryHeavy)
	mux.HandleFunc("POST /cost-reporter", ts.handleCostReporter)
	mux.HandleFunc("GET /health", ts.handleHealth)
	mux.HandleFunc("GET /stats", ts.handleStats)

	ts.srv = &http.Server{
		Addr:              ts.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return ts
}

// Start begins serving requests in a background goroutine.
// If the configured port is in use, it falls back to an OS-assigned port.
func (ts *TestServer) Start() error {
	ln, err := net.Listen("tcp", ts.addr)
	if err != nil {
		// Port in use - try OS-assigned port
		ln, err = net.Listen("tcp", ":0") //nolint:gosec // test server intentionally binds to all interfaces
		if err != nil {
			return fmt.Errorf("test server failed to listen: %w", err)
		}
	}

	// Extract actual port
	actualAddr := ln.Addr().String()
	ts.addr = actualAddr

	go func() {
		_ = ts.srv.Serve(ln)
	}()

	return nil
}

// Close shuts down the test server.
func (ts *TestServer) Close() error {
	return ts.srv.Close()
}

// Addr returns the server listen address.
func (ts *TestServer) Addr() string {
	return ts.addr
}

// URL returns the full URL for the given endpoint path.
func (ts *TestServer) URL(path string) string {
	return fmt.Sprintf("http://%s%s", ts.addr, path)
}

// Snapshot returns a point-in-time copy of server stats.
func (ts *TestServer) Snapshot() ServerStatsSnapshot {
	return ServerStatsSnapshot{
		FastEcho:     ts.stats.FastEcho.Load(),
		SlowProcess:  ts.stats.SlowProcess.Load(),
		VariableLoad: ts.stats.VariableLoad.Load(),
		Flaky:        ts.stats.Flaky.Load(),
		MemoryHeavy:  ts.stats.MemoryHeavy.Load(),
		CostReporter: ts.stats.CostReporter.Load(),
		Total:        ts.stats.Total.Load(),
	}
}

// handleFastEcho responds immediately with the received payload.
// Simulates fast HTTP-dispatch jobs.
func (ts *TestServer) handleFastEcho(w http.ResponseWriter, r *http.Request) {
	ts.stats.FastEcho.Add(1)
	ts.stats.Total.Add(1)

	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"echo":      body,
		"timestamp": time.Now().UnixMilli(),
	})
}

// handleSlowProcess simulates work taking 1-5 seconds.
func (ts *TestServer) handleSlowProcess(w http.ResponseWriter, _ *http.Request) {
	ts.stats.SlowProcess.Add(1)
	ts.stats.Total.Add(1)

	delay := time.Duration(1+rand.IntN(4)) * time.Second //nolint:gosec // non-cryptographic use for load test simulation
	time.Sleep(delay)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"processed": true,
		"delay_ms":  delay.Milliseconds(),
		"timestamp": time.Now().UnixMilli(),
	})
}

// handleVariableLoad generates configurable CPU load.
func (ts *TestServer) handleVariableLoad(w http.ResponseWriter, _ *http.Request) {
	ts.stats.VariableLoad.Add(1)
	ts.stats.Total.Add(1)

	// Simulate variable processing time (100ms-2s)
	delay := time.Duration(100+rand.IntN(1900)) * time.Millisecond //nolint:gosec // non-cryptographic use for load test simulation
	start := time.Now()

	// Do some actual work during the delay
	iterations := 0
	for time.Since(start) < delay {
		// Busy-wait with real computation
		_ = rand.IntN(1000) * rand.IntN(1000) //nolint:gosec // non-cryptographic use for load test simulation
		iterations++
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"iterations": iterations,
		"delay_ms":   time.Since(start).Milliseconds(),
	})
}

// handleFlaky fails ~20% of the time to test retry behavior.
func (ts *TestServer) handleFlaky(w http.ResponseWriter, _ *http.Request) {
	ts.stats.Flaky.Add(1)
	ts.stats.Total.Add(1)

	if rand.IntN(5) == 0 { //nolint:gosec // non-cryptographic use for load test simulation
		http.Error(w, "simulated failure", http.StatusInternalServerError)
		return
	}

	time.Sleep(time.Duration(50+rand.IntN(200)) * time.Millisecond) //nolint:gosec // non-cryptographic use for load test simulation

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"timestamp": time.Now().UnixMilli(),
	})
}

// handleMemoryHeavy allocates memory to simulate heavy responses.
func (ts *TestServer) handleMemoryHeavy(w http.ResponseWriter, _ *http.Request) {
	ts.stats.MemoryHeavy.Add(1)
	ts.stats.Total.Add(1)

	// Generate a large-ish response (~100KB)
	items := make([]map[string]any, 1000)
	for i := range items {
		items[i] = map[string]any{
			"id":    i,
			"value": fmt.Sprintf("item-%d-data-padding-for-size", i),
			"score": rand.Float64() * 100, //nolint:gosec // non-cryptographic use for load test simulation
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"count": len(items),
	})
}

// handleCostReporter simulates a job that reports cost metadata.
func (ts *TestServer) handleCostReporter(w http.ResponseWriter, _ *http.Request) {
	ts.stats.CostReporter.Add(1)
	ts.stats.Total.Add(1)

	time.Sleep(time.Duration(200+rand.IntN(300)) * time.Millisecond) //nolint:gosec // non-cryptographic use for load test simulation

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"cost": map[string]any{
			"provider":          "openai",
			"model":             "gpt-4o",
			"prompt_tokens":     800 + rand.IntN(500),       //nolint:gosec // non-cryptographic use for load test simulation
			"completion_tokens": 200 + rand.IntN(300),       //nolint:gosec // non-cryptographic use for load test simulation
			"total_cost_usd":    0.01 + rand.Float64()*0.05, //nolint:gosec // non-cryptographic use for load test simulation
		},
		"result": "processed",
	})
}

func (ts *TestServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"uptime": time.Since(ts.started).String(),
	})
}

func (ts *TestServer) handleStats(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ts.Snapshot())
}
