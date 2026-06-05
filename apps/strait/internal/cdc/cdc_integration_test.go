//go:build integration

package cdc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/cdc"
	"strait/internal/pubsub"
	"strait/internal/testutil"
)

var (
	redisOnce sync.Once
	testRedis *testutil.TestRedis
	redisErr  error
)

// mustRedis lazily starts a Redis testcontainer the first time it is called.
func mustRedis(t *testing.T) *testutil.TestRedis {
	t.Helper()
	redisOnce.Do(func() {
		testRedis, redisErr = testutil.SetupSharedTestRedis(context.Background(), "cdc")
	})
	require.Nil(t, redisErr)

	t.Cleanup(func() {
		// Container cleanup is handled at process exit; individual tests
		// just need the connection to stay open.
	})
	return testRedis
}

// newRedisPublisher creates a RedisPublisher with a fresh client for test isolation.
func newRedisPublisher(t *testing.T) *pubsub.RedisPublisher {
	t.Helper()
	tr := mustRedis(t)
	client := redis.NewClient(tr.Options())
	t.Cleanup(func() { _ = client.Close() })
	return pubsub.NewRedisPublisher(client)
}

// sequinServer creates a mock Sequin API server that serves messages from the
// provided channel. Calls to /ack and /nack are tracked via the provided maps.
func sequinServer(
	t *testing.T,
	msgCh <-chan []cdc.Message,
	acked *sync.Map,
	nacked *sync.Map,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/receive"):
			var req struct {
				BatchSize int `json:"batch_size"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.BatchSize <= 0 {
				req.BatchSize = 10
			}

			var msgs []cdc.Message
			for range req.BatchSize {
				select {
				case batch := <-msgCh:
					msgs = append(msgs, batch...)
				default:
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": msgs})

		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/ack"):
			var req struct {
				AckIDs []string `json:"ack_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, id := range req.AckIDs {
				acked.Store(id, true)
			}
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/nack"):
			var req struct {
				AckIDs []string `json:"ack_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, id := range req.AckIDs {
				nacked.Store(id, true)
			}
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func hasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func makeMessage(ackID, table string, action cdc.Action, record map[string]any) cdc.Message {
	recordBytes, _ := json.Marshal(record)
	return cdc.Message{
		AckID:  ackID,
		Record: recordBytes,
		Action: action,
		Metadata: cdc.Metadata{
			TableName:       table,
			TableSchema:     "public",
			CommitTimestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// Test: Publishing change events via handlers and verifying Redis delivery

func TestIntegration_HandlerPublishesToRedis(t *testing.T) {
	pub := newRedisPublisher(t)
	ctx := context.Background()

	channel := "cdc:project:proj-001:job_runs"
	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t, err)

	defer sub.Close()

	handler := cdc.NewJobRunHandler(pub, nil)
	msg := makeMessage("ack-1", "job_runs", cdc.ActionInsert, map[string]any{
		"id":         "run-001",
		"job_id":     "job-001",
		"project_id": "proj-001",
		"status":     "running",
	})
	require.NoError(t, handler.
		Handle(ctx, msg))

	select {
	case data := <-sub.Ch:
		var event cdc.ChangeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			require.Failf(t, "test failure",

				"unmarshal event: %v", err)
		}
		if event.Table != "job_runs" {
			assert.Failf(t, "test failure",

				"event.Table = %q, want %q", event.Table, "job_runs")
		}
		if event.Action != cdc.ActionInsert {
			assert.Failf(t, "test failure",

				"event.Action = %q, want %q", event.Action, cdc.ActionInsert)
		}
		if event.Source != "cdc" {
			assert.Failf(t, "test failure",

				"event.Source = %q, want %q", event.Source, "cdc")
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for published event on Redis")
	}
}

// Test: Consumer connects, receives, and dispatches to handlers

func TestIntegration_ConsumerReceivesAndDispatches(t *testing.T) {
	var handled atomic.Int32
	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-10", "job_runs", cdc.ActionInsert, map[string]any{
			"id":         "run-010",
			"job_id":     "job-010",
			"project_id": "proj-010",
			"status":     "queued",
		}),
		makeMessage("ack-11", "job_runs", cdc.ActionUpdate, map[string]any{
			"id":         "run-011",
			"job_id":     "job-011",
			"project_id": "proj-011",
			"status":     "running",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "test-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "test-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, msg cdc.Message) error {
			handled.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(5 * time.Second)
	for handled.Load() < 2 {
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out: handled %d messages, want 2", handled.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)

	if _, ok := acked.Load("ack-10"); !ok {
		assert.Fail(t,

			"ack-10 was not acknowledged")
	}
	if _, ok := acked.Load("ack-11"); !ok {
		assert.Fail(t,

			"ack-11 was not acknowledged")
	}
}

// Test: Consumer nacks messages when handler returns an error

func TestIntegration_ConsumerNacksOnHandlerError(t *testing.T) {
	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-err-1", "job_runs", cdc.ActionInsert, map[string]any{
			"id":         "run-err-1",
			"job_id":     "job-err-1",
			"project_id": "proj-err-1",
			"status":     "queued",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "test-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "test-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			return fmt.Errorf("intentional failure")
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(5 * time.Second)
	for {
		if _, ok := nacked.Load("ack-err-1"); ok {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for nack of ack-err-1")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)
}

// Test: Consumer reconnects after server error (simulated connection drop)

func TestIntegration_ConsumerReconnectsAfterError(t *testing.T) {
	var requestCount atomic.Int32
	var handled atomic.Int32
	var acked sync.Map

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/receive"):
			count := requestCount.Add(1)
			if count <= 2 {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if count == 3 {
				msg := makeMessage("ack-reconnect", "job_runs", cdc.ActionInsert, map[string]any{
					"id":         "run-reconnect",
					"job_id":     "job-reconnect",
					"project_id": "proj-reconnect",
					"status":     "queued",
				})
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []cdc.Message{msg}})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []cdc.Message{}})

		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/ack"):
			var req struct {
				AckIDs []string `json:"ack_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, id := range req.AckIDs {
				acked.Store(id, true)
			}
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/nack"):
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "test-consumer", "token",
		cdc.WithRetryPolicy(nil),
		cdc.WithCircuitBreaker(nil),
	)
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "test-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			handled.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(25 * time.Second)
	for handled.Load() < 1 {
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out: handled %d messages after %d requests, want at least 1",
				handled.Load(), requestCount.Load())
		case <-time.After(100 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)

	if _, ok := acked.Load("ack-reconnect"); !ok {
		assert.Fail(t,

			"ack-reconnect was not acknowledged after reconnection")
	}
	assert.GreaterOrEqual(
		t, requestCount.
			Load(), int32(3))

}

// Test: Multiple consumers on the same channel via Redis pub/sub fan-out

func TestIntegration_MultipleConsumersOnSameChannel(t *testing.T) {
	pub := newRedisPublisher(t)
	ctx := context.Background()

	channel := "cdc:project:proj-multi:job_runs"
	sub1, err := pub.Subscribe(ctx, channel)
	require.NoError(t, err)

	defer sub1.Close()

	sub2, err := pub.Subscribe(ctx, channel)
	require.NoError(t, err)

	defer sub2.Close()

	handler := cdc.NewJobRunHandler(pub, nil)
	msg := makeMessage("ack-multi", "job_runs", cdc.ActionUpdate, map[string]any{
		"id":         "run-multi",
		"job_id":     "job-multi",
		"project_id": "proj-multi",
		"status":     "completed",
	})
	require.NoError(t, handler.
		Handle(ctx, msg))

	for i, sub := range []*pubsub.Subscription{sub1, sub2} {
		select {
		case data := <-sub.Ch:
			var event cdc.ChangeEvent
			if err := json.Unmarshal(data, &event); err != nil {
				require.Failf(t, "test failure",

					"sub%d: unmarshal event: %v", i+1, err)
			}
			if event.Table != "job_runs" {
				assert.Failf(t, "test failure",

					"sub%d: event.Table = %q, want %q", i+1, event.Table, "job_runs")
			}
		case <-time.After(5 * time.Second):
			require.Failf(t, "test failure", "sub%d: timed out waiting for message", i+1)
		}
	}
}

// Test: High-volume event processing through consumer

func TestIntegration_HighVolumeEventProcessing(t *testing.T) {
	const totalMessages = 200
	var handled atomic.Int32
	var acked sync.Map

	msgCh := make(chan []cdc.Message, totalMessages)

	for i := range totalMessages / 10 {
		batch := make([]cdc.Message, 10)
		for j := range 10 {
			idx := i*10 + j
			batch[j] = makeMessage(
				fmt.Sprintf("ack-vol-%d", idx),
				"job_runs",
				cdc.ActionInsert,
				map[string]any{
					"id":         fmt.Sprintf("run-vol-%d", idx),
					"job_id":     fmt.Sprintf("job-vol-%d", idx),
					"project_id": "proj-vol",
					"status":     "queued",
				},
			)
		}
		msgCh <- batch
	}

	ts := sequinServer(t, msgCh, &acked, &acked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "vol-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "vol-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			handled.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(14 * time.Second)
	for handled.Load() < totalMessages {
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out: handled %d/%d messages", handled.Load(), totalMessages)
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)
	assert.EqualValues(t, totalMessages,

		handled.Load())

}

// Test: Event ordering guarantees within a single batch

func TestIntegration_EventOrderingWithinBatch(t *testing.T) {
	var mu sync.Mutex
	var order []string
	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)

	batch := make([]cdc.Message, 5)
	for i := range 5 {
		batch[i] = makeMessage(
			fmt.Sprintf("ack-order-%d", i),
			"job_runs",
			cdc.ActionInsert,
			map[string]any{
				"id":         fmt.Sprintf("run-order-%d", i),
				"job_id":     fmt.Sprintf("job-order-%d", i),
				"project_id": "proj-order",
				"status":     "queued",
				"seq":        i,
			},
		)
	}
	msgCh <- batch

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "order-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "order-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, msg cdc.Message) error {
			var record struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(msg.Record, &record)
			mu.Lock()
			order = append(order, record.ID)
			mu.Unlock()
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(4 * time.Second)
	for {
		mu.Lock()
		n := len(order)
		mu.Unlock()
		if n >= 5 {
			break
		}
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out: received %d/5 messages", n)
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)

	mu.Lock()
	defer mu.Unlock()
	for i := range 5 {
		want := fmt.Sprintf("run-order-%d", i)
		assert.Equal(t, want,

			order[i])

	}
}

// Test: Batch collect via Consumer with real Redis publisher

func TestIntegration_ConsumerBatchCollectPublish(t *testing.T) {
	pub := newRedisPublisher(t)
	ctx := context.Background()

	channel := "cdc:project:proj-batch:job_runs"
	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t, err)

	defer sub.Close()

	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-batch-1", "job_runs", cdc.ActionInsert, map[string]any{
			"id":         "run-batch-1",
			"job_id":     "job-batch-1",
			"project_id": "proj-batch",
			"status":     "queued",
		}),
		makeMessage("ack-batch-2", "job_runs", cdc.ActionUpdate, map[string]any{
			"id":         "run-batch-2",
			"job_id":     "job-batch-2",
			"project_id": "proj-batch",
			"status":     "running",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "batch-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "batch-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	handler := cdc.NewJobRunHandler(pub, nil)
	consumer.RegisterHandler(handler)
	consumer.SetPublisher(pub)

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	go consumer.Run(runCtx)

	received := 0
	deadline := time.After(5 * time.Second)
	for received < 2 {
		select {
		case data := <-sub.Ch:
			var event cdc.ChangeEvent
			if err := json.Unmarshal(data, &event); err != nil {
				require.Failf(t, "test failure",

					"unmarshal event: %v", err)
			}
			if event.Table != "job_runs" {
				assert.Failf(t, "test failure",

					"event.Table = %q, want %q", event.Table, "job_runs")
			}
			received++
		case <-deadline:
			require.Failf(t, "test failure", "timed out: received %d/2 events from Redis", received)
		}
	}

	// Allow the consumer time to ack messages after publishing to Redis.
	// The ack HTTP call happens asynchronously after the publish, so
	// canceling immediately can race with the ack request.
	time.Sleep(500 * time.Millisecond)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)

	if _, ok := acked.Load("ack-batch-1"); !ok {
		assert.Fail(t,

			"ack-batch-1 was not acknowledged")
	}
	if _, ok := acked.Load("ack-batch-2"); !ok {
		assert.Fail(t,

			"ack-batch-2 was not acknowledged")
	}
}

// Test: WebhookReceiver dispatches to handlers with real Redis

func TestIntegration_WebhookReceiverPublishesToRedis(t *testing.T) {
	pub := newRedisPublisher(t)
	ctx := context.Background()

	channel := "cdc:project:proj-wh:job_runs"
	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t, err)

	defer sub.Close()

	receiver := cdc.NewWebhookReceiver(pub, nil)
	handler := cdc.NewJobRunHandler(pub, nil)
	receiver.RegisterHandler(handler)

	msg := makeMessage("ack-wh-1", "job_runs", cdc.ActionUpdate, map[string]any{
		"id":         "run-wh-1",
		"job_id":     "job-wh-1",
		"project_id": "proj-wh",
		"status":     "completed",
	})
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/cdc/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	receiver.ServeHTTP(rec, req)
	require.Equal(t, http.
		StatusOK,
		rec.Code,
	)

	select {
	case data := <-sub.Ch:
		var event cdc.ChangeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			require.Failf(t, "test failure",

				"unmarshal event: %v", err)
		}
		if event.Table != "job_runs" {
			assert.Failf(t, "test failure",

				"event.Table = %q, want %q", event.Table, "job_runs")
		}
		if event.Action != cdc.ActionUpdate {
			assert.Failf(t, "test failure",

				"event.Action = %q, want %q", event.Action, cdc.ActionUpdate)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for webhook event on Redis")
	}
}

// Test: Consumer graceful shutdown waits for in-flight work

func TestIntegration_ConsumerGracefulShutdown(t *testing.T) {
	var handled atomic.Int32
	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-shutdown-1", "job_runs", cdc.ActionInsert, map[string]any{
			"id":         "run-shutdown-1",
			"job_id":     "job-shutdown-1",
			"project_id": "proj-shutdown",
			"status":     "queued",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "shutdown-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "shutdown-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			handled.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)

	deadline := time.After(5 * time.Second)
	for handled.Load() < 1 {
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for message to be handled")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	require.NoError(t, consumer.
		Shutdown(shutdownCtx))

}

// Test: Messages for unregistered tables are acked silently

func TestIntegration_UnregisteredTableAckedSilently(t *testing.T) {
	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-unknown-1", "unknown_table", cdc.ActionInsert, map[string]any{
			"id": "record-1",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "unknown-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "unknown-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			assert.Fail(t,

				"handler should not be called for unknown_table")
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go consumer.Run(ctx)

	deadline := time.After(4 * time.Second)
	for {
		if _, ok := acked.Load("ack-unknown-1"); ok {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for ack of unknown table message")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)
}

// Test: WebhookReceiver with additional handlers

func TestIntegration_WebhookReceiverAdditionalHandlers(t *testing.T) {
	pub := newRedisPublisher(t)

	var additionalHandled atomic.Int32

	receiver := cdc.NewWebhookReceiver(pub, nil)
	handler := cdc.NewJobRunHandler(pub, nil)
	receiver.RegisterHandler(handler)

	receiver.RegisterAdditionalHandler(cdc.HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ cdc.Message) error {
			additionalHandled.Add(1)
			return nil
		},
	})

	msg := makeMessage("ack-add-1", "job_runs", cdc.ActionUpdate, map[string]any{
		"id":         "run-add-1",
		"job_id":     "job-add-1",
		"project_id": "proj-add",
		"status":     "completed",
	})
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/cdc/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	receiver.ServeHTTP(rec, req)
	require.Equal(t, http.
		StatusOK,
		rec.Code,
	)
	assert.EqualValues(t, 1, additionalHandled.
		Load())

}

// Test: Multiple handler types on the same consumer

func TestIntegration_MultipleHandlerTypes(t *testing.T) {
	pub := newRedisPublisher(t)
	ctx := context.Background()

	jobRunCh := "cdc:project:proj-mh:job_runs"
	workflowCh := "cdc:project:proj-mh:workflow_runs"

	subJR, err := pub.Subscribe(ctx, jobRunCh)
	require.NoError(t, err)

	defer subJR.Close()

	subWF, err := pub.Subscribe(ctx, workflowCh)
	require.NoError(t, err)

	defer subWF.Close()

	var acked sync.Map
	var nacked sync.Map

	msgCh := make(chan []cdc.Message, 10)
	msgCh <- []cdc.Message{
		makeMessage("ack-mh-jr", "job_runs", cdc.ActionInsert, map[string]any{
			"id":         "run-mh-1",
			"job_id":     "job-mh-1",
			"project_id": "proj-mh",
			"status":     "queued",
		}),
		makeMessage("ack-mh-wf", "workflow_runs", cdc.ActionInsert, map[string]any{
			"id":          "wf-run-mh-1",
			"workflow_id": "wf-mh-1",
			"project_id":  "proj-mh",
			"status":      "pending",
		}),
	}

	ts := sequinServer(t, msgCh, &acked, &nacked)
	defer ts.Close()

	client := cdc.NewClient(ts.URL, "mh-consumer", "token")
	consumer := cdc.NewConsumer(client, cdc.ConsumerConfig{
		ConsumerName: "mh-consumer",
		BatchSize:    10,
		WaitTimeMs:   100,
	}, nil)

	consumer.RegisterHandler(cdc.NewJobRunHandler(pub, nil))
	consumer.RegisterHandler(cdc.NewWorkflowRunHandler(pub, nil))
	consumer.SetPublisher(pub)

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	go consumer.Run(runCtx)

	select {
	case data := <-subJR.Ch:
		var event cdc.ChangeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			require.Failf(t, "test failure",

				"unmarshal job_runs event: %v", err)
		}
		if event.Table != "job_runs" {
			assert.Failf(t, "test failure",

				"job_runs event.Table = %q, want %q", event.Table, "job_runs")
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for job_runs event")
	}

	select {
	case data := <-subWF.Ch:
		var event cdc.ChangeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			require.Failf(t, "test failure",

				"unmarshal workflow_runs event: %v", err)
		}
		if event.Table != "workflow_runs" {
			assert.Failf(t, "test failure",

				"workflow_runs event.Table = %q, want %q", event.Table, "workflow_runs")
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for workflow_runs event")
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = consumer.Shutdown(shutdownCtx)
}
