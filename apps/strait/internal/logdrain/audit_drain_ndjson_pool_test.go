package logdrain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestEncodeNDJSONBatch_ReturnsIndependentBytes asserts the output slice
// is detached from the pooled buffer — if the pool handed the same
// underlying buffer to another caller before the first caller was done
// with the returned slice, the bytes could be silently overwritten. The
// test triggers two encodes that each consume part of the pool and
// asserts the first caller's slice is unchanged.
func TestEncodeNDJSONBatch_ReturnsIndependentBytes(t *testing.T) {
	t.Parallel()

	ev1 := domain.AuditEvent{
		ID:           "ev-1",
		ProjectID:    "proj-pool-test",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j",
		Details:      json.RawMessage(`{"iteration":1}`),
		CreatedAt:    time.Now().UTC(),
	}
	ev2 := ev1
	ev2.ID = "ev-2"
	ev2.Details = json.RawMessage(`{"iteration":2,"padding":"` + strings.Repeat("x", 4096) + `"}`)

	first, err := encodeNDJSONBatch([]domain.AuditEvent{ev1})
	require.NoError(t, err)

	copyOfFirst := make([]byte, len(first))
	copy(copyOfFirst, first)

	// Second encode returns the pool's buffer to the next caller; if the
	// implementation handed `first` as a view over the pooled bytes,
	// running the second encode would trample it.
	_, err = encodeNDJSONBatch([]domain.AuditEvent{ev2})
	require.NoError(t, err)
	require.Equal(t, string(
		copyOfFirst,
	), string(first))
}

// TestEncodeNDJSONBatch_PooledBufferReuse asserts repeated encodes use
// the pool rather than allocating a fresh buffer every call. We cannot
// observe the pool directly, but we can sanity-check correctness under
// heavy reuse — all events must serialize to the expected JSON shape
// even when the pool is churning.
func TestEncodeNDJSONBatch_PooledBufferReuse(t *testing.T) {
	t.Parallel()

	template := domain.AuditEvent{
		ProjectID:    "proj-pool-reuse",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j",
		CreatedAt:    time.Now().UTC(),
	}

	for i := range 500 {
		ev := template
		ev.ID = "ev-reuse-" + ndjsonItoa(i)
		out, err := encodeNDJSONBatch([]domain.AuditEvent{ev})
		require.NoError(t, err)
		require.Contains(t, string(out), ev.ID)
	}
}

func ndjsonItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
