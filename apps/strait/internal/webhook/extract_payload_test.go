package webhook

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestExtractPayload_PrefersPayloadOverLastError(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		Payload:   []byte(`{"event_key":"x","project_id":"p"}`),
		LastError: `{"trigger_id":"t-shadow"}`,
	}
	got := extractPayload(d)
	require.JSONEq(t, `{"event_key":"x","project_id":"p"}`,

		string(got))
	require.JSONEq(t, `{"trigger_id":"t-shadow"}`,

		d.LastError)
}

func TestExtractPayload_FallsBackToLastErrorWhenJSON(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		LastError: `{"k":"v"}`,
	}
	got := extractPayload(d)
	require.JSONEq(t, `{"k":"v"}`,
		string(got))
	require.Empty(t, d.
		LastError)
}

func TestExtractPayload_FallsBackToMinimalWhenNeither(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:             "del-1",
		EventTriggerID: "trig-1",
	}
	got := extractPayload(d)
	var parsed map[string]any
	require.NoError(t, json.
		Unmarshal(
			got,
			&parsed))
	require.False(t, parsed["trigger_id"] != "trig-1" || parsed["delivery_id"] != "del-1")
}

func TestExtractPayload_PrefersPayloadEvenOverInvalidLastError(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{
		ID:        "del-1",
		Payload:   []byte(`{"event_key":"x"}`),
		LastError: "connection refused",
	}
	got := extractPayload(d)
	require.JSONEq(t, `{"event_key":"x"}`,

		string(got))
}
