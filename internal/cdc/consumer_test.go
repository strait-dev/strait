package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConsumerRegisterHandler(t *testing.T) {
	consumer := NewConsumer(NewClient("http://example.com", "consumer", ""), ConsumerConfig{ConsumerName: "consumer"}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})

	h, ok := consumer.handlers["job_runs"]
	if !ok {
		t.Fatal("handler not registered")
	}
	if h.Table() != "job_runs" {
		t.Fatalf("handler table = %q, want %q", h.Table(), "job_runs")
	}
}

func TestConsumerRegisterMultipleHandlers(t *testing.T) {
	consumer := NewConsumer(NewClient("http://example.com", "consumer", ""), ConsumerConfig{ConsumerName: "consumer"}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "jobs", Fn: func(context.Context, Message) error { return nil }})
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})

	if len(consumer.handlers) != 2 {
		t.Fatalf("len(handlers) = %d, want 2", len(consumer.handlers))
	}
}

func TestConsumerPollRoutesByTableAndAcksSuccess(t *testing.T) {
	var handled atomic.Int32
	var mu sync.Mutex
	var ackIDs [][]string
	var nackCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c1/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c1/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids)
			mu.Unlock()
		case "/api/http_pull_consumers/c1/nack":
			nackCalls++
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c1", "token"), ConsumerConfig{ConsumerName: "c1", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, msg Message) error {
			if msg.AckID != "a1" {
				t.Fatalf("ack id = %q, want %q", msg.AckID, "a1")
			}
			handled.Add(1)
			return nil
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	if handled.Load() != 1 {
		t.Fatalf("handled = %d, want 1", handled.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ackIDs) != 1 || len(ackIDs[0]) != 1 || ackIDs[0][0] != "a1" {
		t.Fatalf("ackIDs = %#v, want [[\"a1\"]]", ackIDs)
	}
	if nackCalls != 0 {
		t.Fatalf("nackCalls = %d, want 0", nackCalls)
	}
}

func TestConsumerPollHandlerFailureNacks(t *testing.T) {
	var mu sync.Mutex
	var ackCalls int
	var nackIDs [][]string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c2/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c2/ack":
			ackCalls++
		case "/api/http_pull_consumers/c2/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids)
			mu.Unlock()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c2", "token"), ConsumerConfig{ConsumerName: "c2", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn:        func(context.Context, Message) error { return errors.New("boom") },
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	if ackCalls != 0 {
		t.Fatalf("ackCalls = %d, want 0", ackCalls)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(nackIDs) != 1 || len(nackIDs[0]) != 1 || nackIDs[0][0] != "a1" {
		t.Fatalf("nackIDs = %#v, want [[\"a1\"]]", nackIDs)
	}
}

func TestConsumerPollUnknownTableAcks(t *testing.T) {
	var mu sync.Mutex
	var ackIDs [][]string
	var nackCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c3/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_name":"unknown_table"}}]}`))
		case "/api/http_pull_consumers/c3/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids)
			mu.Unlock()
		case "/api/http_pull_consumers/c3/nack":
			nackCalls++
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c3", "token"), ConsumerConfig{ConsumerName: "c3", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ackIDs) != 1 || len(ackIDs[0]) != 1 || ackIDs[0][0] != "a1" {
		t.Fatalf("ackIDs = %#v, want [[\"a1\"]]", ackIDs)
	}
	if nackCalls != 0 {
		t.Fatalf("nackCalls = %d, want 0", nackCalls)
	}
}

func TestConsumerPollMixedBatchAckNackSplit(t *testing.T) {
	var mu sync.Mutex
	var ackIDs []string
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c4/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_name":"jobs"}},{"ack_id":"a2","record":{"id":2},"action":"update","metadata":{"table_name":"jobs"}},{"ack_id":"a3","record":{"id":3},"action":"delete","metadata":{"table_name":"unknown"}}]}`))
		case "/api/http_pull_consumers/c4/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/c4/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c4", "token"), ConsumerConfig{ConsumerName: "c4", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "jobs",
		Fn: func(_ context.Context, msg Message) error {
			if msg.AckID == "a2" {
				return errors.New("failed")
			}
			return nil
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ackIDs) != 2 {
		t.Fatalf("len(ackIDs) = %d, want 2", len(ackIDs))
	}
	if !slices.Contains(ackIDs, "a1") || !slices.Contains(ackIDs, "a3") {
		t.Fatalf("ackIDs = %v, want [a1 a3]", ackIDs)
	}
	if len(nackIDs) != 1 || nackIDs[0] != "a2" {
		t.Fatalf("nackIDs = %v, want [a2]", nackIDs)
	}
}

func TestConsumerPollEmptyBatchNoAckNack(t *testing.T) {
	var ackCalls atomic.Int32
	var nackCalls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c5/receive":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/http_pull_consumers/c5/ack":
			ackCalls.Add(1)
		case "/api/http_pull_consumers/c5/nack":
			nackCalls.Add(1)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c5", "token"), ConsumerConfig{ConsumerName: "c5", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	if ackCalls.Load() != 0 {
		t.Fatalf("ackCalls = %d, want 0", ackCalls.Load())
	}
	if nackCalls.Load() != 0 {
		t.Fatalf("nackCalls = %d, want 0", nackCalls.Load())
	}
}

func TestConsumerRunStopsOnContextCancel(t *testing.T) {
	var receiveCalls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/http_pull_consumers/c6/receive" {
			receiveCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c6", "token"), ConsumerConfig{ConsumerName: "c6", BatchSize: 1, WaitTimeMs: 1}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		consumer.Run(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("consumer did not stop after context cancellation")
	}

	if receiveCalls.Load() < 1 {
		t.Fatalf("receiveCalls = %d, want at least 1", receiveCalls.Load())
	}
}

func TestConsumerRunContinuesAfterRecoverableError(t *testing.T) {
	var calls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/c7/receive" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary failure"))
			return
		}

		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c7", "token"), ConsumerConfig{ConsumerName: "c7", BatchSize: 1, WaitTimeMs: 1}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 6200*time.Millisecond)
	defer cancel()
	consumer.Run(ctx)

	if calls.Load() < 2 {
		t.Fatalf("receive calls = %d, want at least 2", calls.Load())
	}
}

func TestConsumer_ProcessMessages_AckError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-ack/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c-ack/ack":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("ack failed"))
		case "/api/http_pull_consumers/c-ack/nack":
			t.Fatal("nack should not be called")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-ack", "token"), ConsumerConfig{ConsumerName: "c-ack", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})

	err := consumer.poll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ack cdc messages") {
		t.Errorf("error = %v, want ack context", err)
	}
}

func TestConsumerPoll_ReceiveErrorWrapped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-receive/receive":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("receive failed"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-receive", "token"), ConsumerConfig{ConsumerName: "c-receive", BatchSize: 10, WaitTimeMs: 1}, slog.Default())

	err := consumer.poll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "receive cdc messages") {
		t.Errorf("error = %v, want receive context", err)
	}
}

func TestConsumerPoll_NackErrorWrapped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-nack/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c-nack/ack":
			t.Fatal("ack should not be called")
		case "/api/http_pull_consumers/c-nack/nack":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nack failed"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-nack", "token"), ConsumerConfig{ConsumerName: "c-nack", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return errors.New("boom") }})

	err := consumer.poll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nack cdc messages") {
		t.Errorf("error = %v, want nack context", err)
	}
}

func decodeAckIDs(t *testing.T, r *http.Request) []string {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
	}
	if got := r.Header.Get("Authorization"); got == "" {
		t.Fatal("Authorization header not set")
	}

	var body struct {
		AckIDs []string `json:"ack_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body.AckIDs
}
