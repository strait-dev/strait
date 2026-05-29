package cdc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestClaimDedupe_LostSharedClaimDoesNotPoisonLocalCache locks in the contract
// that a node which loses the authoritative shared claim must not retain a local
// dedupe entry. Without this, the losing node responds 200 without installing the
// releaseDedupe defer, so a stale seen[key] lingers for the full TTL and silently
// drops a redelivery routed here after the winning node fails and releases.
func TestClaimDedupe_LostSharedClaimDoesNotPoisonLocalCache(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	shared := NewSharedDedupeStore(rdb, time.Hour)

	wr := NewWebhookReceiver(nil, nil,
		WithWebhookDedupeTTL(time.Hour),
		WithWebhookSharedDedupe(shared),
	)

	msg := Message{
		AckID:    "ack-lost",
		Record:   json.RawMessage(`{"id":"run-1"}`),
		Action:   ActionUpdate,
		Metadata: Metadata{TableName: "job_runs", IdempotencyKey: "idem-lost"},
	}
	ctx := context.Background()
	key := wr.dedupeKey(msg)
	sharedKey := "cdc_webhook:" + key

	// Another node wins the authoritative claim first.
	won, err := shared.Claim(ctx, sharedKey)
	if err != nil {
		t.Fatalf("seeding shared claim: %v", err)
	}
	if !won {
		t.Fatal("expected the seeding node to win the shared claim")
	}

	// This node loses the shared claim.
	gotKey, claimed := wr.claimDedupe(ctx, msg)
	if gotKey != key {
		t.Fatalf("claimDedupe key = %q, want %q", gotKey, key)
	}
	if claimed {
		t.Fatal("expected claimed=false when another node holds the shared claim")
	}

	// The local cache must not retain a key this node never committed to process.
	wr.seenMu.Lock()
	_, poisoned := wr.seen[key]
	wr.seenMu.Unlock()
	if poisoned {
		t.Fatal("losing node poisoned its local dedupe cache with an unprocessed key")
	}

	// Winning node fails and releases; a redelivery routed here must still win.
	shared.Release(ctx, sharedKey)
	gotKey, claimed = wr.claimDedupe(ctx, msg)
	if gotKey != key {
		t.Fatalf("redelivery claimDedupe key = %q, want %q", gotKey, key)
	}
	if !claimed {
		t.Fatal("redelivery after release must be processed, but local cache suppressed it")
	}
}
