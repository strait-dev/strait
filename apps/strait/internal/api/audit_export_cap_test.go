package api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestStreamAuditJSON_EmitsCapMarker verifies that the JSON streaming
// path stops at maxExportRows and emits a trailing _capped marker.
func TestStreamAuditJSON_EmitsCapMarker(t *testing.T) {
	orig := maxExportRows
	maxExportRows = 5
	t.Cleanup(func() { maxExportRows = orig })

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := 0; i < 20; i++ {
				ev := &domain.AuditEvent{
					ID:        "ev-" + itoaBench(i),
					ProjectID: "proj-1",
					Action:    domain.AuditActionJobCreated,
					CreatedAt: time.Now(),
				}
				if err := fn(ev); err != nil {
					return err
				}
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	var buf bytes.Buffer
	exported, capped, err := srv.streamAuditJSON(context.Background(), &buf, nil, false,
		"proj-1", "", "", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("streamAuditJSON: %v", err)
	}
	if !capped {
		t.Error("expected capped=true")
	}
	if exported != 5 {
		t.Errorf("exported = %d, want 5", exported)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("parse json: %v\nbody: %s", err, buf.String())
	}
	if len(parsed) != 6 {
		t.Fatalf("parsed len = %d, want 6 (5 rows + cap marker)", len(parsed))
	}
	last := parsed[len(parsed)-1]
	if capped, _ := last["_capped"].(bool); !capped {
		t.Errorf("last element missing _capped marker: %v", last)
	}
}

// TestStreamAuditNDJSON_EmitsCapMarker same for ndjson path.
func TestStreamAuditNDJSON_EmitsCapMarker(t *testing.T) {
	orig := maxExportRows
	maxExportRows = 3
	t.Cleanup(func() { maxExportRows = orig })

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := 0; i < 10; i++ {
				ev := &domain.AuditEvent{
					ID:     "ev-" + itoaBench(i),
					Action: domain.AuditActionJobCreated,
				}
				if err := fn(ev); err != nil {
					return err
				}
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	var buf bytes.Buffer
	exported, capped, err := srv.streamAuditNDJSON(context.Background(), &buf, nil, false,
		"proj-1", "", "", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("streamAuditNDJSON: %v", err)
	}
	if !capped {
		t.Error("expected capped=true")
	}
	if exported != 3 {
		t.Errorf("exported = %d, want 3", exported)
	}
}

// itoaBench is a local int-to-string helper.
func itoaBench(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
