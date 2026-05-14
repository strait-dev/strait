package webhook

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

func TestExtractPayload_PrefersPayloadOverLastError(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		Payload:   []byte(`{"event_key":"x","project_id":"p"}`),
		LastError: `{"trigger_id":"t-shadow"}`,
	}
	got := extractPayload(d)
	if string(got) != `{"event_key":"x","project_id":"p"}` {
		t.Fatalf("payload = %s, want canonical Payload (not LastError)", string(got))
	}
	if d.LastError != `{"trigger_id":"t-shadow"}` {
		t.Fatalf("LastError must not be mutated when Payload is used; got %q", d.LastError)
	}
}

func TestExtractPayload_FallsBackToLastErrorWhenJSON(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		LastError: `{"k":"v"}`,
	}
	got := extractPayload(d)
	if string(got) != `{"k":"v"}` {
		t.Fatalf("payload = %s, want LastError contents", string(got))
	}
	if d.LastError != "" {
		t.Fatalf("LastError should be cleared after lift-out, got %q", d.LastError)
	}
}

func TestExtractPayload_FallsBackToMinimalWhenNeither(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:             "del-1",
		EventTriggerID: "trig-1",
	}
	got := extractPayload(d)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("minimal payload not valid JSON: %v", err)
	}
	if parsed["trigger_id"] != "trig-1" || parsed["delivery_id"] != "del-1" {
		t.Fatalf("unexpected minimal payload: %s", string(got))
	}
}

func TestExtractPayload_PrefersPayloadEvenOverInvalidLastError(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		Payload:   []byte(`{"event_key":"x"}`),
		LastError: "connection refused",
	}
	got := extractPayload(d)
	if string(got) != `{"event_key":"x"}` {
		t.Fatalf("payload = %s, want canonical Payload", string(got))
	}
}
