package api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamAuditJSON_EmitsCapMarker verifies that the JSON streaming
// path stops at maxExportRows and emits a trailing _capped marker.
func TestStreamAuditJSON_EmitsCapMarker(t *testing.T) {
	const cap int64 = 5

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := range 20 {
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
		"proj-1", "", "", time.Now().Add(-time.Hour), time.Now(), cap)
	require.NoError(t, err)
	assert.True(t, capped)
	assert.Equal(t, 5, exported)

	var parsed []map[string]any
	require.NoError(t, json.
		Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed,
		6)

	last := parsed[len(parsed)-1]
	if capped, _ := last["_capped"].(bool); !capped {
		assert.Failf(t, "test failure",

			"last element missing _capped marker: %v", last)
	}
}

// TestStreamAuditNDJSON_EmitsCapMarker same for ndjson path.
func TestStreamAuditNDJSON_EmitsCapMarker(t *testing.T) {
	const cap int64 = 3

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := range 10 {
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
		"proj-1", "", "", time.Now().Add(-time.Hour), time.Now(), cap)
	require.NoError(t, err)
	assert.True(t, capped)
	assert.Equal(t, 3, exported)
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
