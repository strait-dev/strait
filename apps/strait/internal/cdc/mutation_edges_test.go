package cdc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestHandlersWithSharedDedupeNilReceiver(t *testing.T) {
	t.Parallel()

	require.Nil(t, (*AnalyticsHandler)(nil).WithSharedDedupe(nil))
	require.Nil(t, (*AuditHandler)(nil).WithSharedDedupe(nil))
	require.Nil(t, (*SLOHandler)(nil).WithSharedDedupe(nil))
}

func TestDecodeWorkflowStepRunStatusRecordInvalidJSON(t *testing.T) {
	t.Parallel()

	id, record, version, err := decodeWorkflowStepRunStatusRecord(json.RawMessage(`{"id":`))
	require.Error(t, err)
	require.Empty(t, id)
	require.Nil(t, record)
	require.Zero(t, version)
}

func TestRecentDedupeForgetMiddleEntryCompactsOrder(t *testing.T) {
	t.Parallel()

	dedupe := newRecentDedupe(3)
	require.True(t, dedupe.Remember("a"))
	require.True(t, dedupe.Remember("b"))
	require.True(t, dedupe.Remember("c"))

	dedupe.Forget("b")
	require.True(t, dedupe.Remember("b"))

	require.True(t, dedupe.Remember("d"))
	require.False(t, dedupe.Remember("b"))
	require.True(t, dedupe.Remember("a"))
}

func TestHexNibbleBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input byte
		want  byte
	}{
		{input: '0', want: 0},
		{input: '9', want: 9},
		{input: 'a', want: 10},
		{input: 'f', want: 15},
		{input: 'A', want: 10},
		{input: 'F', want: 15},
		{input: '/', want: invalidHexNibble},
		{input: ':', want: invalidHexNibble},
		{input: '`', want: invalidHexNibble},
		{input: 'g', want: invalidHexNibble},
		{input: '@', want: invalidHexNibble},
		{input: 'G', want: invalidHexNibble},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, hexNibble(tt.input))
	}
}

func TestWebhookReceiverReleaseDedupeReleasesSharedClaim(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	shared := NewSharedDedupeStore(rdb, time.Hour)
	wr := NewWebhookReceiver(nil, nil,
		WithWebhookDedupeTTL(time.Hour),
		WithWebhookSharedDedupe(shared),
	)

	ctx := context.Background()
	const key = "idem-release"
	wr.seenMu.Lock()
	wr.seen[key] = time.Now().Add(time.Hour)
	wr.seenMu.Unlock()

	claimed, err := shared.Claim(ctx, "cdc_webhook:"+key)
	require.NoError(t, err)
	require.True(t, claimed)

	wr.releaseDedupe(ctx, key)

	wr.seenMu.Lock()
	_, stillSeen := wr.seen[key]
	wr.seenMu.Unlock()
	require.False(t, stillSeen)

	claimed, err = shared.Claim(ctx, "cdc_webhook:"+key)
	require.NoError(t, err)
	require.True(t, claimed)
}
