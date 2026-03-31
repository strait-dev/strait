package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// -- eventRequest routing tests.

func TestCallbackEventRequest_Checkpoint(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, body, terminal, err := c.eventRequest(RuntimeEvent{
		Type:  RuntimeEventCheckpoint,
		State: json.RawMessage(`{"cursor":1}`),
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/checkpoint" {
		t.Fatalf("path = %q", path)
	}
	if terminal {
		t.Fatal("checkpoint should not be terminal")
	}
	m := body.(map[string]any)
	if m["source"] != "agents_runtime" {
		t.Fatalf("source = %v", m["source"])
	}
}

func TestCallbackEventRequest_Usage(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, body, terminal, err := c.eventRequest(RuntimeEvent{
		Type:             RuntimeEventUsage,
		Provider:         "openai",
		Model:            "gpt-5.4",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CostMicrousd:     300,
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/usage" {
		t.Fatalf("path = %q", path)
	}
	if terminal {
		t.Fatal("usage should not be terminal")
	}
	m := body.(map[string]any)
	if m["provider"] != "openai" {
		t.Fatalf("provider = %v", m["provider"])
	}
	if m["model"] != "gpt-5.4" {
		t.Fatalf("model = %v", m["model"])
	}
}

func TestCallbackEventRequest_ToolCall(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, body, terminal, err := c.eventRequest(RuntimeEvent{
		Type:       RuntimeEventToolCall,
		ToolName:   "search",
		Input:      json.RawMessage(`{"query":"test"}`),
		Output:     json.RawMessage(`{"results":[]}`),
		DurationMs: 120,
		Status:     "completed",
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/tool-call" {
		t.Fatalf("path = %q", path)
	}
	if terminal {
		t.Fatal("tool_call should not be terminal")
	}
	m := body.(map[string]any)
	if m["tool_name"] != "search" {
		t.Fatalf("tool_name = %v", m["tool_name"])
	}
}

func TestCallbackEventRequest_Stream(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, body, _, err := c.eventRequest(RuntimeEvent{
		Type:     RuntimeEventStream,
		Chunk:    "hello ",
		StreamID: "stream-1",
		Done:     false,
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/stream" {
		t.Fatalf("path = %q", path)
	}
	m := body.(map[string]any)
	if m["stream_id"] != "stream-1" {
		t.Fatalf("stream_id = %v", m["stream_id"])
	}
}

func TestCallbackEventRequest_StreamEmptyIDDefaultsToDefault(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	_, body, _, err := c.eventRequest(RuntimeEvent{Type: RuntimeEventStream, StreamID: ""})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	m := body.(map[string]any)
	if m["stream_id"] != "default" {
		t.Fatalf("stream_id = %v, want default", m["stream_id"])
	}
}

func TestCallbackEventRequest_Complete(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, _, terminal, err := c.eventRequest(RuntimeEvent{
		Type:   RuntimeEventComplete,
		Result: json.RawMessage(`{"answer":42}`),
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/complete" {
		t.Fatalf("path = %q", path)
	}
	if !terminal {
		t.Fatal("complete should be terminal")
	}
}

func TestCallbackEventRequest_Fail(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	path, body, terminal, err := c.eventRequest(RuntimeEvent{
		Type:  RuntimeEventFail,
		Error: "out of memory",
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if path != "/fail" {
		t.Fatalf("path = %q", path)
	}
	if !terminal {
		t.Fatal("fail should be terminal")
	}
	m := body.(map[string]any)
	if m["error"] != "out of memory" {
		t.Fatalf("error = %v", m["error"])
	}
}

func TestCallbackEventRequest_UnsupportedType(t *testing.T) {
	t.Parallel()
	c := &HTTPCallbackClient{}
	_, _, _, err := c.eventRequest(RuntimeEvent{Type: "totally_unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported event type")
	}
}

// -- postJSON tests.

func TestCallbackPostJSON_EmptyBaseURL(t *testing.T) {
	t.Parallel()
	c := NewHTTPCallbackClient("", nil)
	err := c.postJSON(context.Background(), "run-1", "token", "/checkpoint", map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty base URL")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %q, want 'not configured'", err.Error())
	}
}

func TestCallbackPostJSON_ServerReturns200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	err := c.postJSON(context.Background(), "run-1", "test-token", "/checkpoint", map[string]any{"state": "ok"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCallbackPostJSON_ServerReturns500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	err := c.postJSON(context.Background(), "run-1", "token", "/fail", map[string]any{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %q, want to contain '500'", err.Error())
	}
}

func TestCallbackPostJSON_CorrectAuthHeader(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	_ = c.postJSON(context.Background(), "run-1", "my-secret-token", "/usage", map[string]any{})
	if gotAuth != "Bearer my-secret-token" {
		t.Fatalf("Authorization = %q, want Bearer my-secret-token", gotAuth)
	}
}

func TestCallbackPostJSON_CorrectContentType(t *testing.T) {
	t.Parallel()
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	_ = c.postJSON(context.Background(), "run-1", "token", "/usage", map[string]any{})
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotCT)
	}
}

func TestCallbackPostJSON_ConnectionRefused(t *testing.T) {
	t.Parallel()
	c := NewHTTPCallbackClient("http://127.0.0.1:1", &http.Client{Timeout: time.Second})
	err := c.postJSON(context.Background(), "run-1", "token", "/usage", map[string]any{})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

// -- Send terminal/non-terminal tests.

func TestCallbackSend_CheckpointIsNonTerminal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	terminal, err := c.Send(context.Background(), "run-1", "token", RuntimeEvent{Type: RuntimeEventCheckpoint})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if terminal {
		t.Fatal("checkpoint should return terminal=false")
	}
}

func TestCallbackSend_CompleteIsTerminal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	terminal, err := c.Send(context.Background(), "run-1", "token", RuntimeEvent{Type: RuntimeEventComplete})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !terminal {
		t.Fatal("complete should return terminal=true")
	}
}

func TestCallbackSend_FailIsTerminal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPCallbackClient(srv.URL, srv.Client())
	terminal, err := c.Send(context.Background(), "run-1", "token", RuntimeEvent{Type: RuntimeEventFail, Error: "boom"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !terminal {
		t.Fatal("fail should return terminal=true")
	}
}

// -- rawMessageBody tests.

func TestRawMessageBody_Nil(t *testing.T) {
	t.Parallel()
	got := rawMessageBody(nil, `{}`)
	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", got)
	}
	if string(raw) != "{}" {
		t.Fatalf("got %q, want {}", string(raw))
	}
}

func TestRawMessageBody_Empty(t *testing.T) {
	t.Parallel()
	got := rawMessageBody(json.RawMessage{}, `null`)
	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", got)
	}
	if string(raw) != "null" {
		t.Fatalf("got %q, want null", string(raw))
	}
}

func TestRawMessageBody_Valid(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"key":"value"}`)
	got := rawMessageBody(input, `{}`)
	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", got)
	}
	if string(raw) != `{"key":"value"}` {
		t.Fatalf("got %q", string(raw))
	}
}

// -- Fuzz test.

func FuzzCallbackEventRequest(f *testing.F) {
	f.Add("checkpoint", "chunk-data", "error-msg", "tool-name")
	f.Add("usage", "", "", "")
	f.Add("complete", "", "", "")
	f.Add("fail", "", "boom", "")
	f.Add("stream", "hello", "", "")
	f.Add("unknown_type", "", "", "")

	f.Fuzz(func(t *testing.T, eventType, chunk, errMsg, toolName string) {
		c := &HTTPCallbackClient{}
		_, _, _, _ = c.eventRequest(RuntimeEvent{
			Type:     RuntimeEventType(eventType),
			Chunk:    chunk,
			Error:    errMsg,
			ToolName: toolName,
		})
	})
}

var _ io.Reader // ensure io is used
