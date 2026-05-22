package cdc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"strait/internal/clickhouse"
	"strait/internal/pubsub"
)

func TestDeepSecConsumerPoll_RunsAdditionalHandlersForPullConsumer(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var sideEffects int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/deepsec/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":"r1","project_id":"p1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/deepsec/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case "/api/http_pull_consumers/deepsec/nack":
			t.Fatal("unexpected nack")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "deepsec", "token"), ConsumerConfig{ConsumerName: "deepsec", BatchSize: 10, WaitTimeMs: 1}, nil)
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})
	consumer.RegisterAdditionalHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error {
		sideEffects++
		return nil
	}})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if sideEffects != 1 {
		t.Fatalf("sideEffects = %d, want 1", sideEffects)
	}
	if len(ackIDs) != 1 || ackIDs[0] != "a1" {
		t.Fatalf("ackIDs = %v, want [a1]", ackIDs)
	}
}

func TestDeepSecConsumerPoll_NacksAdditionalHandlerFailure(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackCalls int
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/deepsec-fail/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":"r1","project_id":"p1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/deepsec-fail/ack":
			mu.Lock()
			ackCalls++
			mu.Unlock()
		case "/api/http_pull_consumers/deepsec-fail/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "deepsec-fail", "token"), ConsumerConfig{ConsumerName: "deepsec-fail", BatchSize: 10, WaitTimeMs: 1}, nil)
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})
	consumer.RegisterAdditionalHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error {
		return errors.New("durable side effect failed")
	}})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if ackCalls != 0 {
		t.Fatalf("ackCalls = %d, want 0", ackCalls)
	}
	if len(nackIDs) != 1 || nackIDs[0] != "a1" {
		t.Fatalf("nackIDs = %v, want [a1]", nackIDs)
	}
}

func TestDeepSecConsumerPoll_BatchAdditionalFailureNacksAfterPublish(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackCalls int
	var nackIDs []string
	pub := &deepSecTrackingPublisher{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/deepsec-batch/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":"r1","project_id":"p1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/deepsec-batch/ack":
			mu.Lock()
			ackCalls++
			mu.Unlock()
		case "/api/http_pull_consumers/deepsec-batch/nack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			nackIDs = append(nackIDs, ids...)
			mu.Unlock()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "deepsec-batch", "token"), ConsumerConfig{ConsumerName: "deepsec-batch", BatchSize: 10, WaitTimeMs: 1}, nil)
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&mockCollectableHandler{
		table: "job_runs",
		collectFn: func(context.Context, Message) (*pubsub.PubSubMessage, error) {
			return &pubsub.PubSubMessage{Channel: "cdc:job_runs", Data: []byte(`{"id":"r1"}`)}, nil
		},
	})
	consumer.RegisterAdditionalHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error {
		return errors.New("durable side effect failed")
	}})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if pub.batchCalls != 1 {
		t.Fatalf("PublishBatch calls = %d, want 1", pub.batchCalls)
	}
	if ackCalls != 0 {
		t.Fatalf("ackCalls = %d, want 0", ackCalls)
	}
	if len(nackIDs) != 1 || nackIDs[0] != "a1" {
		t.Fatalf("nackIDs = %v, want [a1]", nackIDs)
	}
}

func TestDeepSecAnalyticsHandler_AcceptsJSONBTags(t *testing.T) {
	t.Parallel()

	exp := clickhouse.NewTestExporter()
	h := NewAnalyticsHandler(exp, nil)
	msg := Message{
		AckID:  "ack-1",
		Action: ActionUpdate,
		Record: []byte(`{
			"id":"run-1",
			"job_id":"job-1",
			"project_id":"p1",
			"status":"completed",
			"attempt":1,
			"tags":{"team":"platform","risk":["billing","cdc"]},
			"created_at":"2026-03-26T09:59:00Z",
			"started_at":"2026-03-26T10:00:00Z",
			"finished_at":"2026-03-26T10:00:05Z"
		}`),
		Metadata: Metadata{TableName: "job_runs"},
	}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord")
	}
	if rec.Tags != `{"team":"platform","risk":["billing","cdc"]}` {
		t.Fatalf("Tags = %q", rec.Tags)
	}
}

type deepSecTrackingPublisher struct {
	batchCalls int
}

func (p *deepSecTrackingPublisher) Publish(context.Context, string, []byte) error { return nil }
func (p *deepSecTrackingPublisher) PublishBatch(context.Context, []pubsub.PubSubMessage) error {
	p.batchCalls++
	return nil
}
func (p *deepSecTrackingPublisher) Subscribe(context.Context, string) (*pubsub.Subscription, error) {
	return nil, nil
}
func (p *deepSecTrackingPublisher) Close() error { return nil }
