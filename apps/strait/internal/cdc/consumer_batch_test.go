package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"strait/internal/pubsub"
)

// collectableHandlerFunc implements CollectableHandler for testing.
type collectableHandlerFunc struct {
	tableName string
	handleFn  func(ctx context.Context, msg Message) error
	collectFn func(ctx context.Context, msg Message) (*pubsub.PubSubMessage, error)
}

func (h *collectableHandlerFunc) Table() string { return h.tableName }
func (h *collectableHandlerFunc) Handle(ctx context.Context, msg Message) error {
	if h.handleFn != nil {
		return h.handleFn(ctx, msg)
	}
	return nil
}
func (h *collectableHandlerFunc) Collect(ctx context.Context, msg Message) (*pubsub.PubSubMessage, error) {
	if h.collectFn != nil {
		return h.collectFn(ctx, msg)
	}
	return nil, nil
}

// trackingPublisher records all PublishBatch calls.
type trackingPublisher struct {
	mu           sync.Mutex
	batchCalls   int
	totalMsgs    int
	publishErr   error
	publishCalls int
}

func (p *trackingPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	p.mu.Lock()
	p.publishCalls++
	p.mu.Unlock()
	return nil
}

func (p *trackingPublisher) PublishBatch(_ context.Context, msgs []pubsub.PubSubMessage) error {
	p.mu.Lock()
	p.batchCalls++
	p.totalMsgs += len(msgs)
	err := p.publishErr
	p.mu.Unlock()
	return err
}

func TestConsumerPoll_BatchCollection_AcksAfterPublish(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/batch/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"r1","project_id":"p1"},"action":"insert","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}},
				{"ack_id":"a2","record":{"id":"r2","project_id":"p1"},"action":"update","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}}
			]}`))
		case "/api/http_pull_consumers/batch/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/batch/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		}
	}))
	defer ts.Close()

	pub := &trackingPublisher{}
	consumer := NewConsumer(NewClient(ts.URL, "batch", "token"), ConsumerConfig{ConsumerName: "batch", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&collectableHandlerFunc{
		tableName: "job_runs",
		collectFn: func(_ context.Context, msg Message) (*pubsub.PubSubMessage, error) {
			return &pubsub.PubSubMessage{
				Channel: "cdc:project:p1:job_runs",
				Data:    msg.Record,
			}, nil
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll error: %v", err)
	}

	pub.mu.Lock()
	if pub.batchCalls != 1 {
		t.Errorf("batch calls = %d, want 1", pub.batchCalls)
	}
	if pub.totalMsgs != 2 {
		t.Errorf("total messages = %d, want 2", pub.totalMsgs)
	}
	pub.mu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if len(ackIDs) != 2 {
		t.Errorf("ack IDs = %v, want 2 items", ackIDs)
	}
	if !slices.Contains(ackIDs, "a1") || !slices.Contains(ackIDs, "a2") {
		t.Errorf("ack IDs should contain a1 and a2, got %v", ackIDs)
	}
	if len(nackIDs) != 0 {
		t.Errorf("nack IDs should be empty, got %v", nackIDs)
	}
}

func TestConsumerPoll_BatchPublishFailure_AcksProjectionOnlyMessage(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/bfail/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"r1","project_id":"p1"},"action":"insert","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}}
			]}`))
		case "/api/http_pull_consumers/bfail/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/bfail/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		}
	}))
	defer ts.Close()

	pub := &trackingPublisher{publishErr: errors.New("redis down")}
	consumer := NewConsumer(NewClient(ts.URL, "bfail", "token"), ConsumerConfig{ConsumerName: "bfail", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&collectableHandlerFunc{
		tableName: "job_runs",
		collectFn: func(_ context.Context, msg Message) (*pubsub.PubSubMessage, error) {
			return &pubsub.PubSubMessage{Channel: "ch", Data: msg.Record}, nil
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Projection publish is best-effort. Without a durable additional handler
	// failure, the message is ACKed to avoid redelivery amplification.
	if len(ackIDs) != 1 || ackIDs[0] != "a1" {
		t.Errorf("ack IDs should contain a1 on publish failure, got %v", ackIDs)
	}
	if len(nackIDs) != 0 {
		t.Errorf("nack IDs should be empty on projection publish failure, got %v", nackIDs)
	}
}

func TestConsumerPoll_BatchPublishFailure_NacksAdditionalHandlerFailure(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/bfail_side/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"r1","project_id":"p1"},"action":"insert","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}}
			]}`))
		case "/api/http_pull_consumers/bfail_side/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/bfail_side/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		}
	}))
	defer ts.Close()

	pub := &trackingPublisher{publishErr: errors.New("redis down")}
	consumer := NewConsumer(NewClient(ts.URL, "bfail_side", "token"), ConsumerConfig{ConsumerName: "bfail_side", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&collectableHandlerFunc{
		tableName: "job_runs",
		collectFn: func(_ context.Context, msg Message) (*pubsub.PubSubMessage, error) {
			return &pubsub.PubSubMessage{Channel: "ch", Data: msg.Record}, nil
		},
	})
	consumer.RegisterAdditionalHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(context.Context, Message) error {
			return errors.New("durable side effect failed")
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ackIDs) != 0 {
		t.Errorf("ack IDs should be empty on durable handler failure, got %v", ackIDs)
	}
	if len(nackIDs) != 1 || nackIDs[0] != "a1" {
		t.Errorf("nack IDs should contain a1 on durable handler failure, got %v", nackIDs)
	}
}

func TestConsumerPoll_CollectError_NacksMessage(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/cerr/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"bad json","action":"insert","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}}
			]}`))
		case "/api/http_pull_consumers/cerr/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/cerr/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		}
	}))
	defer ts.Close()

	pub := &trackingPublisher{}
	consumer := NewConsumer(NewClient(ts.URL, "cerr", "token"), ConsumerConfig{ConsumerName: "cerr", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&collectableHandlerFunc{
		tableName: "job_runs",
		collectFn: func(_ context.Context, _ Message) (*pubsub.PubSubMessage, error) {
			return nil, errors.New("collect failed")
		},
	})

	// The receive call will fail to parse, so poll will return error.
	_ = consumer.poll(context.Background())
}

func BenchmarkConsumerRunAdditionalHandlers(b *testing.B) {
	consumer := NewConsumer(nil, ConsumerConfig{ConsumerName: "bench"}, slog.Default())
	consumer.RegisterAdditionalHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(context.Context, Message) error {
			return nil
		},
	})
	msg := Message{
		AckID:    "a1",
		Record:   json.RawMessage(`{"id":"r1","project_id":"p1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}

	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := consumer.runAdditionalHandlers(ctx, msg); err != nil {
			b.Fatal(err)
		}
	}
}

func TestConsumerPoll_MixedHandlers_BatchAndInline(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var inlineHandled int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/mix/receive":
			// Two messages: one for batch handler, one for inline handler.
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"r1","project_id":"p1"},"action":"insert","metadata":{"table_name":"job_runs","commit_timestamp":"2026-03-18T00:00:00Z"}},
				{"ack_id":"a2","record":{"id":"w1"},"action":"update","metadata":{"table_name":"workflows","commit_timestamp":"2026-03-18T00:00:00Z"}}
			]}`))
		case "/api/http_pull_consumers/mix/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/mix/nack":
			// Should not be called.
			t.Error("nack should not be called")
		}
	}))
	defer ts.Close()

	pub := &trackingPublisher{}
	consumer := NewConsumer(NewClient(ts.URL, "mix", "token"), ConsumerConfig{ConsumerName: "mix", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.SetPublisher(pub)

	// Batch handler for job_runs (implements CollectableHandler).
	consumer.RegisterHandler(&collectableHandlerFunc{
		tableName: "job_runs",
		collectFn: func(_ context.Context, msg Message) (*pubsub.PubSubMessage, error) {
			return &pubsub.PubSubMessage{Channel: "ch", Data: msg.Record}, nil
		},
	})

	// Inline handler for workflows (only implements Handler via HandlerFunc).
	consumer.RegisterHandler(HandlerFunc{
		TableName: "workflows",
		Fn: func(_ context.Context, _ Message) error {
			mu.Lock()
			inlineHandled++
			mu.Unlock()
			return nil
		},
	})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll error: %v", err)
	}

	pub.mu.Lock()
	if pub.batchCalls != 1 {
		t.Errorf("batch calls = %d, want 1 (for job_runs)", pub.batchCalls)
	}
	if pub.totalMsgs != 1 {
		t.Errorf("batch messages = %d, want 1", pub.totalMsgs)
	}
	pub.mu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if inlineHandled != 1 {
		t.Errorf("inline handled = %d, want 1 (for workflows)", inlineHandled)
	}
	if len(ackIDs) != 2 {
		t.Errorf("ack IDs = %v, want 2 (both batch and inline)", ackIDs)
	}
}

func TestConsumerSetPublisher(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://example.com", "c", ""), ConsumerConfig{ConsumerName: "c"}, slog.Default())
	if consumer.publisher != nil {
		t.Fatal("publisher should be nil initially")
	}

	pub := &trackingPublisher{}
	consumer.SetPublisher(pub)
	if consumer.publisher == nil {
		t.Fatal("publisher should be set after SetPublisher")
	}
}

// decodeAckIDs is defined in consumer_test.go.
