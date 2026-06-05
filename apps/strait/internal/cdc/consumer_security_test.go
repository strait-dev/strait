package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/pubsub"

	"github.com/stretchr/testify/require"
)

// TestCDC_MalformedChangeEvent verifies that garbage CDC data results in
// handler errors without panicking.
func TestCDC_MalformedChangeEvent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		record string
	}{
		{"empty_object", `{}`},
		{"null_record", `null`},
		{"invalid_json", `{not json at all`},
		{"numeric_record", `42`},
		{"array_record", `[1,2,3]`},
		{"null_bytes", "{\"id\":\"\x00\"}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := NewJobRunHandler(nil, slog.Default())
			msg := Message{
				AckID:  "ack-1",
				Record: json.RawMessage(tc.record),
				Action: ActionInsert,
				Metadata: Metadata{
					TableName:       "job_runs",
					CommitTimestamp: time.Now().Format(time.RFC3339),
				},
			}

			// Must not panic. Some cases return error, some succeed with empty fields.
			_ = handler.Handle(context.Background(), msg)
		})
	}
}

// TestCDC_EventWithWrongProjectID verifies that a CDC event referencing a
// non-existent project ID is handled without error by the handler (it just
// publishes to the derived channel).
func TestCDC_EventWithWrongProjectID(t *testing.T) {
	t.Parallel()

	var publishedChannel string
	pub := &mockCDCPublisher{
		publishFn: func(_ context.Context, channel string, _ []byte) error {
			publishedChannel = channel
			return nil
		},
	}

	handler := NewJobRunHandler(pub, slog.Default())
	msg := Message{
		AckID:  "ack-1",
		Record: json.RawMessage(`{"id":"run-1","job_id":"j-1","project_id":"proj-nonexistent","status":"completed"}`),
		Action: ActionUpdate,
		Metadata: Metadata{
			TableName:       "job_runs",
			CommitTimestamp: time.Now().Format(time.RFC3339),
		},
	}
	require.NoError(t, handler.Handle(context.
		Background(), msg))
	require.Equal(t, "cdc:project:proj-nonexistent:job_runs",

		publishedChannel,
	)
}

// TestCDC_ConsumerReconnection verifies that the consumer retries polling
// after a connection failure.
func TestCDC_ConsumerReconnection(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		switch {
		case r.URL.Path == "/api/http_pull_consumers/c1/receive" && count == 1:
			// First call fails.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"connection reset"}`))
		case r.URL.Path == "/api/http_pull_consumers/c1/receive" && count > 1:
			// Subsequent calls succeed.
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "c1", "token",
		WithRetryPolicy(nil),
		WithCircuitBreaker(nil),
	)
	consumer := NewConsumer(client, ConsumerConfig{
		ConsumerName: "c1",
		BatchSize:    10,
		WaitTimeMs:   1,
	}, slog.Default())

	// First poll should return an error.
	err := consumer.poll(context.Background())
	require.Error(t, err)

	// Second poll should succeed.
	err = consumer.poll(context.Background())
	require.NoError(t, err)
}

// TestCDC_MessageOrderingGuarantee verifies that messages within a single
// poll batch are processed in order.
func TestCDC_MessageOrderingGuarantee(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var order []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c1/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"1"},"action":"insert","metadata":{"table_name":"test_table"}},
				{"ack_id":"a2","record":{"id":"2"},"action":"insert","metadata":{"table_name":"test_table"}},
				{"ack_id":"a3","record":{"id":"3"},"action":"insert","metadata":{"table_name":"test_table"}}
			]}`))
		case "/api/http_pull_consumers/c1/ack":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "c1", "token",
		WithRetryPolicy(nil),
		WithCircuitBreaker(nil),
	)
	consumer := NewConsumer(client, ConsumerConfig{
		ConsumerName: "c1",
		BatchSize:    10,
		WaitTimeMs:   1,
	}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "test_table",
		Fn: func(_ context.Context, msg Message) error {
			mu.Lock()
			order = append(order, msg.AckID)
			mu.Unlock()
			return nil
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		order, 3)
	require.False(t, order[0] != "a1" ||
		order[1] != "a2" ||
		order[2] != "a3")
}

// TestCDC_DuplicateEvent verifies that the same event delivered twice
// is processed twice (idempotency is the handler's responsibility).
func TestCDC_DuplicateEvent(t *testing.T) {
	t.Parallel()

	var handleCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c1/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"run-1","job_id":"j-1","project_id":"p-1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs","idempotency_key":"idem-1"}},
				{"ack_id":"a2","record":{"id":"run-1","job_id":"j-1","project_id":"p-1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs","idempotency_key":"idem-1"}}
			]}`))
		case "/api/http_pull_consumers/c1/ack":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "c1", "token",
		WithRetryPolicy(nil),
		WithCircuitBreaker(nil),
	)
	consumer := NewConsumer(client, ConsumerConfig{
		ConsumerName: "c1",
		BatchSize:    10,
		WaitTimeMs:   1,
	}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ Message) error {
			handleCount.Add(1)
			return nil
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))
	require.EqualValues(t, 2, handleCount.
		Load())
}

// TestCDC_BackpressureHandling verifies that when a handler returns errors,
// the consumer nacks the failed messages.
func TestCDC_BackpressureHandling(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var nackIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c1/receive":
			_, _ = w.Write([]byte(`{"data":[
				{"ack_id":"a1","record":{"id":"1"},"action":"insert","metadata":{"table_name":"slow_table"}},
				{"ack_id":"a2","record":{"id":"2"},"action":"insert","metadata":{"table_name":"slow_table"}},
				{"ack_id":"a3","record":{"id":"3"},"action":"insert","metadata":{"table_name":"slow_table"}}
			]}`))
		case "/api/http_pull_consumers/c1/ack":
			w.WriteHeader(http.StatusOK)
		case "/api/http_pull_consumers/c1/nack":
			var req struct {
				AckIDs []string `json:"ack_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			nackIDs = append(nackIDs, req.AckIDs...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "c1", "token",
		WithRetryPolicy(nil),
		WithCircuitBreaker(nil),
	)
	consumer := NewConsumer(client, ConsumerConfig{
		ConsumerName: "c1",
		BatchSize:    10,
		WaitTimeMs:   1,
	}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "slow_table",
		Fn: func(_ context.Context, _ Message) error {
			return errors.New("consumer overwhelmed")
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		nackIDs, 3)
}

// FuzzCDCEvent fuzzes CDC payloads to ensure the handler does not panic.
func FuzzCDCEvent(f *testing.F) {
	f.Add([]byte(`{"id":"run-1","job_id":"j-1","project_id":"p-1","status":"completed"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))
	f.Add([]byte{0x00, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		handler := NewJobRunHandler(nil, slog.Default())
		msg := Message{
			AckID:  "fuzz-ack",
			Record: json.RawMessage(data),
			Action: ActionInsert,
			Metadata: Metadata{
				TableName:       "job_runs",
				CommitTimestamp: "2024-01-01T00:00:00Z",
			},
		}
		// Must not panic.
		_ = handler.Handle(context.Background(), msg)
	})
}

// mockCDCPublisher implements EventPublisher for tests.
type mockCDCPublisher struct {
	publishFn      func(ctx context.Context, channel string, data []byte) error
	publishBatchFn func(ctx context.Context, messages []pubsub.PubSubMessage) error
}

func (m *mockCDCPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return nil
}

func (m *mockCDCPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	if m.publishBatchFn != nil {
		return m.publishBatchFn(ctx, messages)
	}
	return nil
}
