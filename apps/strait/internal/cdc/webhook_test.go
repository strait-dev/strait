package cdc

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/pubsub"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type webhookMockHandler struct {
	table   string
	handled []Message
	err     error
}

func (m *webhookMockHandler) Table() string { return m.table }
func (m *webhookMockHandler) Handle(_ context.Context, msg Message) error {
	m.handled = append(m.handled, msg)
	return m.err
}

type webhookMockCollectHandler struct {
	webhookMockHandler
	collected []Message
	pubMsg    *pubsub.PubSubMessage
}

func (m *webhookMockCollectHandler) Collect(_ context.Context, msg Message) (*pubsub.PubSubMessage, error) {
	m.collected = append(m.collected, msg)
	return m.pubMsg, m.err
}

type blockingWebhookHandler struct {
	table   string
	entered chan struct{}
	release chan struct{}
	count   atomic.Int64
}

func (h *blockingWebhookHandler) Table() string { return h.table }
func (h *blockingWebhookHandler) Handle(_ context.Context, _ Message) error {
	h.count.Add(1)
	select {
	case h.entered <- struct{}{}:
	default:
	}
	<-h.release
	return nil
}

func makeWebhookRequest(msg Message) *http.Request {
	body, _ := json.Marshal(msg)
	return httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader(body))
}

func signWebhookBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func BenchmarkWebhookReceiverVerifySignature(b *testing.B) {
	secret := "cdc-webhook-secret"
	body := []byte(`{"ack_id":"ack-1","record":{"id":"run-1","project_id":"project-1","status":"completed"},"action":"update","metadata":{"table_name":"job_runs","idempotency_key":"idem-1"}}`)
	signature := signWebhookBody(secret, body)
	wr := NewWebhookReceiver(nil, nil, WithWebhookSecret(secret))
	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", nil)
	req.Header.Set("X-Sequin-Signature", signature)

	b.ReportAllocs()
	for b.Loop() {
		if !wr.verifySignature(req, body) {
			b.Fatal("signature rejected")
		}
	}
}

func TestWebhookReceiverVerifySignatureAcceptsUppercaseHex(t *testing.T) {
	t.Parallel()

	secret := "cdc-webhook-secret"
	body := []byte(`{"ack_id":"ack-1","action":"update","metadata":{"table_name":"job_runs"}}`)
	wr := NewWebhookReceiver(nil, nil, WithWebhookSecret(secret))
	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", nil)
	req.Header.Set("X-Sequin-Signature", "sha256="+strings.ToUpper(strings.TrimPrefix(signWebhookBody(secret, body), "sha256=")))

	require.True(t, wr.verifySignature(req, body))
}

func TestWebhookReceiver_DispatchesByTable(t *testing.T) {
	t.Parallel()
	h := &webhookMockHandler{table: "job_runs"}
	wr := NewWebhookReceiver(nil, nil)
	wr.RegisterHandler(h)

	msg := Message{
		AckID:  "ack-1",
		Record: json.RawMessage(`{"id":"run-1"}`),
		Action: ActionUpdate,
		Metadata: Metadata{
			TableName: "job_runs",
		},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
	require.Len(t, h.handled,
		1)
}

func TestWebhookReceiver_UnknownTable_Returns200(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	msg := Message{
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "unknown_table"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
}

func TestWebhookReceiver_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusBadRequest,
		rr.Code)
}

func TestWebhookReceiver_HandlerError_Returns500(t *testing.T) {
	t.Parallel()
	h := &webhookMockHandler{table: "job_runs", err: errors.New("db error")}
	wr := NewWebhookReceiver(nil, nil)
	wr.RegisterHandler(h)

	msg := Message{
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusInternalServerError,
		rr.Code,
	)
}

func TestWebhookReceiver_CollectAndPublish(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := &webhookMockCollectHandler{
		webhookMockHandler: webhookMockHandler{table: "job_runs"},
		pubMsg:             &pubsub.PubSubMessage{Channel: "cdc:project:p1:job_runs", Data: []byte(`{"test":true}`)},
	}
	wr := NewWebhookReceiver(pub, nil)
	wr.RegisterHandler(h)

	msg := Message{
		Record:   json.RawMessage(`{"id":"run-1","project_id":"p1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
	require.Len(t, h.collected,
		1)
	require.Len(t, pub.calls,
		1)
	assert.Equal(t, "cdc:project:p1:job_runs",

		pub.calls[0].channel)
}

func TestWebhookReceiver_PublishFailureStillAcksProjection(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error {
		return errors.New("redis down")
	}}
	h := &webhookMockCollectHandler{
		webhookMockHandler: webhookMockHandler{table: "job_runs"},
		pubMsg:             &pubsub.PubSubMessage{Channel: "cdc:project:p1:job_runs", Data: []byte(`{"test":true}`)},
	}
	wr := NewWebhookReceiver(pub, nil)
	wr.RegisterHandler(h)

	msg := Message{
		Record:   json.RawMessage(`{"id":"run-1","project_id":"p1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
	require.Len(t, h.collected,
		1)
	require.Len(t, pub.calls,
		1)
}

func TestWebhookReceiver_PublishFailureStillRetriesAdditionalHandlerFailure(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error {
		return errors.New("redis down")
	}}
	primary := &webhookMockCollectHandler{
		webhookMockHandler: webhookMockHandler{table: "job_runs"},
		pubMsg:             &pubsub.PubSubMessage{Channel: "cdc:project:p1:job_runs", Data: []byte(`{"test":true}`)},
	}
	sideEffect := &webhookMockHandler{table: "job_runs", err: errors.New("durable side effect failed")}
	wr := NewWebhookReceiver(pub, nil)
	wr.RegisterHandler(primary)
	wr.RegisterAdditionalHandler(sideEffect)

	msg := Message{
		Record:   json.RawMessage(`{"id":"run-1","project_id":"p1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusInternalServerError,
		rr.Code,
	)
	require.Len(t, sideEffect.
		handled, 1)
}

func TestDeepSecWebhookReceiver_RejectsNonPost(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/internal/cdc/webhook", nil)
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusMethodNotAllowed,
		rr.Code)
}

func TestDeepSecWebhookReceiver_RejectsOversizedBody(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", strings.NewReader(strings.Repeat("a", maxWebhookBodyBytes+1)))
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusRequestEntityTooLarge,
		rr.
			Code)
}

func TestDeepSecWebhookReceiver_RejectsInvalidAction(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	msg := Message{
		Action:   Action("drop_table"),
		Metadata: Metadata{TableName: "job_runs"},
	}
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusBadRequest,
		rr.Code)
}

func TestDeepSecWebhookReceiver_RejectsReadAndEmptyActions(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		action Action
	}{
		{name: "read", action: ActionRead},
		{name: "empty", action: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wr := NewWebhookReceiver(nil, nil)
			msg := Message{
				Action:   tt.action,
				Metadata: Metadata{TableName: "job_runs"},
			}
			rr := httptest.NewRecorder()
			wr.ServeHTTP(rr, makeWebhookRequest(msg))
			require.Equal(t, http.
				StatusBadRequest,
				rr.Code)
		})
	}
}

func TestDeepSecWebhookReceiver_VerifiesHMACSignature(t *testing.T) {
	t.Parallel()
	secret := "cdc-webhook-secret"
	h := &webhookMockHandler{table: "job_runs"}
	wr := NewWebhookReceiver(nil, nil, WithWebhookSecret(secret))
	wr.RegisterHandler(h)

	msg := Message{
		AckID:    "ack-signed",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs", IdempotencyKey: "idem-signed"},
	}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusUnauthorized,
		rr.Code)

	req = httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader(body))
	req.Header.Set("X-Sequin-Signature", "sha256=bad")
	rr = httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusUnauthorized,
		rr.Code)

	req = httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader(body))
	req.Header.Set("X-Sequin-Signature", signWebhookBody(secret, body))
	rr = httptest.NewRecorder()
	wr.ServeHTTP(rr, req)
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
	require.Len(t, h.handled,
		1)
}

func TestDeepSecWebhookReceiver_SuppressesDuplicateIdempotencyKey(t *testing.T) {
	t.Parallel()
	h := &webhookMockHandler{table: "job_runs"}
	wr := NewWebhookReceiver(nil, nil, WithWebhookDedupeTTL(time.Hour))
	wr.RegisterHandler(h)

	msg := Message{
		AckID:    "ack-1",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs", IdempotencyKey: "idem-1"},
	}

	for range 2 {
		rr := httptest.NewRecorder()
		wr.ServeHTTP(rr, makeWebhookRequest(msg))
		require.Equal(t, http.
			StatusOK, rr.Code,
		)
	}
	require.Len(t, h.handled,
		1)
}

func TestDeepSecWebhookReceiver_SuppressesConcurrentDuplicateIdempotencyKey(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	h := &blockingWebhookHandler{
		table:   "job_runs",
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	wr := NewWebhookReceiver(nil, nil, WithWebhookDedupeTTL(time.Hour))
	wr.RegisterHandler(h)

	msg := Message{
		AckID:    "ack-concurrent",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs", IdempotencyKey: "idem-concurrent"},
	}

	firstDone := make(chan int, 1)
	concWG.Go(func() {
		rr := httptest.NewRecorder()
		wr.ServeHTTP(rr, makeWebhookRequest(msg))
		firstDone <- rr.Code
	})

	select {
	case <-h.entered:
	case <-time.After(time.Second):
		require.Fail(t, "first webhook did not enter handler")
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)

	close(h.release)
	select {
	case code := <-firstDone:
		if code != http.StatusOK {
			require.Failf(t, "test failure",

				"first webhook status = %d, want 200", code)
		}
	case <-time.After(time.Second):
		require.Fail(t, "first webhook did not finish")
	}
	require.EqualValues(t, 1, h.
		count.Load())
}

func TestDeepSecWebhookReceiver_DoesNotMarkFailedDeliverySeen(t *testing.T) {
	t.Parallel()
	h := &webhookMockHandler{table: "job_runs", err: errors.New("db unavailable")}
	wr := NewWebhookReceiver(nil, nil, WithWebhookDedupeTTL(time.Hour))
	wr.RegisterHandler(h)

	msg := Message{
		AckID:    "ack-fail",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs", IdempotencyKey: "idem-fail"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusInternalServerError,
		rr.Code,
	)

	h.err = nil
	rr = httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusOK, rr.Code,
	)
	require.Len(t, h.handled,
		2)
}

func TestDeepSecWebhookReceiver_AdditionalHandlerFailureRetriesDelivery(t *testing.T) {
	t.Parallel()
	primary := &webhookMockHandler{table: "job_runs"}
	sideEffect := &webhookMockHandler{table: "job_runs", err: errors.New("audit write failed")}
	wr := NewWebhookReceiver(nil, nil)
	wr.RegisterHandler(primary)
	wr.RegisterAdditionalHandler(sideEffect)

	msg := Message{
		AckID:    "ack-side-effect",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs"},
	}
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))
	require.Equal(t, http.
		StatusInternalServerError,
		rr.Code,
	)
}
