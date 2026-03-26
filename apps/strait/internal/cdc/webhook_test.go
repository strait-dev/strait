package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/pubsub"
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

func makeWebhookRequest(msg Message) *http.Request {
	body, _ := json.Marshal(msg)
	return httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader(body))
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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(h.handled) != 1 {
		t.Fatalf("expected 1 handled message, got %d", len(h.handled))
	}
}

func TestWebhookReceiver_UnknownTable_Returns200(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	msg := Message{
		Metadata: Metadata{TableName: "unknown_table"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown table, got %d", rr.Code)
	}
}

func TestWebhookReceiver_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/internal/cdc/webhook", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhookReceiver_HandlerError_Returns500(t *testing.T) {
	t.Parallel()
	h := &webhookMockHandler{table: "job_runs", err: errors.New("db error")}
	wr := NewWebhookReceiver(nil, nil)
	wr.RegisterHandler(h)

	msg := Message{
		Metadata: Metadata{TableName: "job_runs"},
	}

	rr := httptest.NewRecorder()
	wr.ServeHTTP(rr, makeWebhookRequest(msg))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(h.collected) != 1 {
		t.Fatalf("expected 1 collected message, got %d", len(h.collected))
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.calls))
	}
	if pub.calls[0].channel != "cdc:project:p1:job_runs" {
		t.Errorf("unexpected channel: %s", pub.calls[0].channel)
	}
}
