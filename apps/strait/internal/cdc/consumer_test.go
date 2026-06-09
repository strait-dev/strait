package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/pubsub"

	"github.com/sourcegraph/conc"
)

type countingLogHandler struct {
	warnCount atomic.Int32
}

func (h *countingLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *countingLogHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Level >= slog.LevelWarn {
		h.warnCount.Add(1)
	}
	return nil
}

func (h *countingLogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *countingLogHandler) WithGroup(string) slog.Handler {
	return h
}

func TestConsumerRegisterHandler(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://example.com", "consumer", ""), ConsumerConfig{ConsumerName: "consumer"}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})

	h, ok := consumer.handlers["job_runs"]
	assert.True(
		t, ok)
	assert.Equal(t, "job_runs", h.
		Table())
}

func TestConsumerRegisterMultipleHandlers(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://example.com", "consumer", ""), ConsumerConfig{ConsumerName: "consumer"}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "jobs", Fn: func(context.Context, Message) error { return nil }})
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})
	assert.Len(t,
		consumer.handlers,
		2)
}

func TestConsumerBatchPublishFailureSentryScope(t *testing.T) {
	t.Parallel()

	consumer := NewConsumer(NewClient("http://example.com", "cdc-consumer", ""), ConsumerConfig{ConsumerName: "cdc-consumer"}, slog.Default())
	scope := sentry.NewScope()
	consumer.applyBatchPublishFailureSentryScope(scope, 3)

	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	assert.Equal(t, "cdc", event.
		Tags["subsystem"])
	assert.Equal(t, "cdc-consumer",
		event.Tags["consumer"])
	assert.Equal(t, "publish_batch",
		event.
			Tags["operation"])
	assert.EqualValues(t, 3, event.Contexts["cdc.batch"]["batch_count"])
}

func TestConsumerPollRoutesByTableAndAcksSuccess(t *testing.T) {
	t.Parallel()
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
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c1", "token"), ConsumerConfig{ConsumerName: "c1", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, msg Message) error {
			assert.Equal(t, "a1", msg.AckID)

			handled.Add(1)
			return nil
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))
	assert.EqualValues(t, 1, handled.Load())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, [][]string{{"a1"}}, ackIDs)
	assert.Equal(t, 0, nackCalls)
}

func TestConsumerPollHandlerFailureNacks(t *testing.T) {
	t.Parallel()
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
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c2", "token"), ConsumerConfig{ConsumerName: "c2", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn:        func(context.Context, Message) error { return errors.New("boom") },
	})
	require.NoError(t, consumer.poll(context.
		Background()))
	assert.Equal(t, 0, ackCalls)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, [][]string{{"a1"}}, nackIDs)
}

func TestConsumerPollUnknownTableAcks(t *testing.T) {
	t.Parallel()
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
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c3", "token"), ConsumerConfig{ConsumerName: "c3", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, [][]string{{"a1"}}, ackIDs)
	assert.Equal(t, 0, nackCalls)
}

func TestConsumerPollEmptyTableAcksWithoutWarn(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var ackIDs [][]string
	var nackCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-empty/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a-empty","record":{"id":1},"action":"insert","metadata":{"table_name":""}}]}`))
		case "/api/http_pull_consumers/c-empty/ack":
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids)
			mu.Unlock()
		case "/api/http_pull_consumers/c-empty/nack":
			nackCalls++
		default:
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	logs := &countingLogHandler{}
	consumer := NewConsumer(
		NewClient(ts.URL, "c-empty", "token"),
		ConsumerConfig{ConsumerName: "c-empty", BatchSize: 10, WaitTimeMs: 1},
		slog.New(logs),
	)
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, len(ackIDs) !=
		1 || len(ackIDs[0]) !=
		1 || ackIDs[0][0] != "a-empty",
	)
	assert.Equal(t, 0, nackCalls)
	assert.EqualValues(t, 0, logs.warnCount.
		Load())
}

func TestConsumerPollMixedBatchAckNackSplit(t *testing.T) {
	t.Parallel()
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
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
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
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t,
		ackIDs, 2)
	assert.False(t, !slices.Contains(ackIDs,
		"a1") || !slices.Contains(ackIDs, "a3"))
	assert.False(t, len(nackIDs) !=
		1 || nackIDs[0] !=
		"a2")
}

func TestConsumerPollEmptyBatchNoAckNack(t *testing.T) {
	t.Parallel()
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
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c5", "token"), ConsumerConfig{ConsumerName: "c5", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	require.NoError(t, consumer.poll(context.
		Background()))
	assert.EqualValues(t, 0, ackCalls.Load())
	assert.EqualValues(t, 0, nackCalls.
		Load())
}

func TestConsumerRunStopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	var receiveCalls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/http_pull_consumers/c6/receive" {
			receiveCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		assert.Failf(t, "test failure",

			"unexpected path: %s", r.URL.Path)
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c6", "token"), ConsumerConfig{ConsumerName: "c6", BatchSize: 1, WaitTimeMs: 1}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		consumer.Run(ctx)
		close(done)
	})

	deadline := time.After(2 * time.Second)
	for receiveCalls.Load() < 1 {
		select {
		case <-deadline:
			assert.Fail(t, "consumer never called /receive within 2s")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		assert.Fail(t, "consumer did not stop after context cancellation")
	}
	assert.GreaterOrEqual(t, receiveCalls.
		Load(), int32(1))
}

func TestConsumer_Shutdown_Idle(t *testing.T) {
	t.Parallel()

	consumer := NewConsumer(NewClient("http://example.com", "consumer", "token"), ConsumerConfig{ConsumerName: "consumer"}, slog.Default())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	assert.NoError(t, consumer.Shutdown(shutdownCtx))
}

func TestConsumer_Shutdown_WaitsForPoll(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	pollStarted := make(chan struct{})
	allowPollExit := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/c-shutdown/receive",

			r.URL.Path)

		select {
		case <-pollStarted:
		default:
			close(pollStarted)
		}

		select {
		case <-allowPollExit:
			_, _ = w.Write([]byte(`{"data":[]}`))
		case <-r.Context().Done():
			return
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(
		NewClient(ts.URL, "c-shutdown", "token"),
		ConsumerConfig{ConsumerName: "c-shutdown", BatchSize: 1, WaitTimeMs: 1},
		slog.Default(),
	)

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	runDone := make(chan struct{})
	concWG.Go(func() {
		consumer.Run(runCtx)
		close(runDone)
	})

	select {
	case <-pollStarted:
	case <-time.After(time.Second):
		assert.Fail(t, "poll did not start")
	}

	shutdownDone := make(chan error, 1)
	concWG.Go(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- consumer.Shutdown(shutdownCtx)
	})

	select {
	case err := <-shutdownDone:
		assert.Failf(t, "test failure", "Shutdown returned early with err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowPollExit)

	select {
	case err := <-shutdownDone:
		if err != nil {
			assert.Failf(t, "test failure",

				"Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Shutdown did not return after poll completed")
	}

	select {
	case <-runDone:
	case <-time.After(time.Second):
		assert.Fail(t, "consumer did not stop after shutdown")
	}
}

func TestConsumerRunContinuesAfterRecoverableError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/c7/receive",

			r.URL.Path,
		)

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
	assert.GreaterOrEqual(t, calls.
		Load(),
		int32(2))
}

func TestConsumer_ProcessMessages_AckError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-ack/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c-ack/ack":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("ack failed"))
		case "/api/http_pull_consumers/c-ack/nack":
			assert.Fail(t, "nack should not be called")
		default:
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-ack", "token"), ConsumerConfig{ConsumerName: "c-ack", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return nil }})

	err := consumer.poll(context.Background())
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "ack cdc messages",
	)
}

func TestConsumerPoll_ReceiveErrorWrapped(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-receive/receive":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("receive failed"))
		default:
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-receive", "token"), ConsumerConfig{ConsumerName: "c-receive", BatchSize: 10, WaitTimeMs: 1}, slog.Default())

	err := consumer.poll(context.Background())
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "receive cdc messages")
}

func TestConsumerPoll_NackErrorWrapped(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-nack/receive":
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case "/api/http_pull_consumers/c-nack/ack":
			assert.Fail(t, "ack should not be called")
		case "/api/http_pull_consumers/c-nack/nack":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nack failed"))
		default:
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(NewClient(ts.URL, "c-nack", "token"), ConsumerConfig{ConsumerName: "c-nack", BatchSize: 10, WaitTimeMs: 1}, slog.Default())
	consumer.RegisterHandler(HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error { return errors.New("boom") }})

	err := consumer.poll(context.Background())
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "nack cdc messages")
}

func decodeAckIDs(t *testing.T, r *http.Request) []string {
	t.Helper()
	assert.Equal(t, http.MethodPost,
		r.Method,
	)
	assert.Equal(t, "application/json",
		r.Header.
			Get("Content-Type"))
	assert.NotEmpty(t, r.Header.
		Get("Authorization"))

	var body struct {
		AckIDs []string `json:"ack_ids"`
	}
	assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

	return body.AckIDs
}

func TestConsumerDefaultWaitTimeMs(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://localhost", "c1", "token"), ConsumerConfig{ConsumerName: "c1"}, nil)
	assert.Equal(t, 1000, consumer.
		config.WaitTimeMs,
	)
}

func TestConsumerDefaultBatchSize(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://localhost", "c1", "token"), ConsumerConfig{ConsumerName: "c1"}, nil)
	assert.Equal(t, 200, consumer.
		config.BatchSize,
	)
}

func TestConsumerPoll_LargeBatchSize_HonoredAndRoutedInOnePoll(t *testing.T) {
	t.Parallel()

	const wantBatchSize = 200
	const messageCount = 200

	var (
		handled     atomic.Int32
		seenBatchSz atomic.Int32
		ackedIDs    sync.Map
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http_pull_consumers/c-bs/receive":
			var req struct {
				BatchSize int `json:"batch_size"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				assert.Failf(t, "test failure",

					"decode receive body: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			seenBatchSz.Store(int32(req.BatchSize))

			// Return exactly batch_size messages in one response.
			var sb strings.Builder
			sb.WriteString(`{"data":[`)
			for i := range messageCount {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(`{"ack_id":"a`)
				sb.WriteString(strconv.Itoa(i))
				sb.WriteString(`","record":{"id":`)
				sb.WriteString(strconv.Itoa(i))
				sb.WriteString(`},"action":"insert","metadata":{"table_name":"job_runs"}}`)
			}
			sb.WriteString(`]}`)
			_, _ = w.Write([]byte(sb.String()))
		case "/api/http_pull_consumers/c-bs/ack":
			ids := decodeAckIDs(t, r)
			for _, id := range ids {
				ackedIDs.Store(id, struct{}{})
			}
		case "/api/http_pull_consumers/c-bs/nack":
			assert.Failf(t, "test failure", "unexpected nack call")
		default:
			assert.Failf(t, "test failure", "unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(
		NewClient(ts.URL, "c-bs", "token"),
		ConsumerConfig{ConsumerName: "c-bs", BatchSize: wantBatchSize, WaitTimeMs: 1},
		slog.Default(),
	)
	consumer.RegisterHandler(HandlerFunc{
		TableName: "job_runs",
		Fn: func(_ context.Context, _ Message) error {
			handled.Add(1)
			return nil
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))
	assert.Equal(t, wantBatchSize,
		int(seenBatchSz.
			Load()))
	assert.Equal(t, messageCount,
		int(handled.
			Load()))

	ackedCount := 0
	ackedIDs.Range(func(_, _ any) bool { ackedCount++; return true })
	assert.Equal(t, messageCount,
		ackedCount,
	)
}

func TestConsumer_ShutdownDuringErrorBackoff(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	var receiveCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/receive"):
			receiveCalls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary failure"))
		default:
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(
		NewClient(ts.URL, "c-backoff", "token"),
		ConsumerConfig{ConsumerName: "c-backoff", BatchSize: 1, WaitTimeMs: 1},
		slog.Default(),
	)

	runDone := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	concWG.Go(func() {
		consumer.Run(ctx)
		close(runDone)
	})

	// Wait until at least one poll error occurs (consumer enters the 5s backoff select).
	deadline := time.After(2 * time.Second)
	for receiveCalls.Load() < 1 {
		select {
		case <-deadline:
			assert.Fail(t, "receive was never called")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Shutdown while consumer is in 5s error backoff.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	require.NoError(t, consumer.Shutdown(shutdownCtx))

	select {
	case <-runDone:
	case <-time.After(10 * time.Second):
		assert.Fail(t, "consumer did not stop after shutdown during error backoff")
	}
}

type mockCollectableHandler struct {
	table     string
	collectFn func(ctx context.Context, msg Message) (*pubsub.PubSubMessage, error)
}

func (h *mockCollectableHandler) Table() string { return h.table }
func (h *mockCollectableHandler) Handle(_ context.Context, _ Message) error {
	return nil
}
func (h *mockCollectableHandler) Collect(ctx context.Context, msg Message) (*pubsub.PubSubMessage, error) {
	return h.collectFn(ctx, msg)
}

type consumerMockPublisher struct {
	mu             sync.Mutex
	publishBatchFn func(ctx context.Context, msgs []pubsub.PubSubMessage) error
	batchCalls     int
}

func (p *consumerMockPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (p *consumerMockPublisher) PublishBatch(ctx context.Context, msgs []pubsub.PubSubMessage) error {
	p.mu.Lock()
	p.batchCalls++
	p.mu.Unlock()
	if p.publishBatchFn != nil {
		return p.publishBatchFn(ctx, msgs)
	}
	return nil
}

func TestConsumer_CollectReturnsNilMessage_AcksWithoutPublish(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var ackIDs []string
	var nackCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/receive"):
			_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"update","metadata":{"table_name":"job_runs"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/ack"):
			ids := decodeAckIDs(t, r)
			mu.Lock()
			ackIDs = append(ackIDs, ids...)
			mu.Unlock()
		case strings.HasSuffix(r.URL.Path, "/nack"):
			mu.Lock()
			nackCalls++
			mu.Unlock()
		}
	}))
	defer ts.Close()

	consumer := NewConsumer(
		NewClient(ts.URL, "c-nil-collect", "token"),
		ConsumerConfig{ConsumerName: "c-nil-collect", BatchSize: 10, WaitTimeMs: 1},
		slog.Default(),
	)

	pub := &consumerMockPublisher{}
	consumer.SetPublisher(pub)
	consumer.RegisterHandler(&mockCollectableHandler{
		table: "job_runs",
		collectFn: func(_ context.Context, _ Message) (*pubsub.PubSubMessage, error) {
			return nil, nil
		},
	})
	require.NoError(t, consumer.poll(context.
		Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, len(ackIDs) !=
		1 || ackIDs[0] != "a1",
	)
	assert.Equal(t, 0, nackCalls)

	pub.mu.Lock()
	defer pub.mu.Unlock()
	assert.Equal(t, 0, pub.batchCalls)
}

func TestConsumer_RegisterHandler_Nil(t *testing.T) {
	t.Parallel()
	consumer := NewConsumer(NewClient("http://example.com", "c", ""), ConsumerConfig{ConsumerName: "c"}, nil)
	consumer.RegisterHandler(nil)
	assert.Empty(t,
		consumer.handlers)
}
