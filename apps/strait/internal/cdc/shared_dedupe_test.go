package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSharedDedupe_SuppressesAcrossInstances(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	shared := NewSharedDedupeStore(rdb, time.Minute)
	a := newRecentDedupe(16).WithShared(shared, nil)
	b := newRecentDedupe(16).WithShared(shared, nil)

	if !a.Remember("cdc:key-1") {
		t.Fatal("first Remember() = false, want true")
	}
	if b.Remember("cdc:key-1") {
		t.Fatal("second instance Remember() = true, want false")
	}
}

func TestSharedDedupe_TTLExpiryAllowsReprocessing(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	shared := NewSharedDedupeStore(rdb, time.Second)

	if ok, err := shared.Claim(context.Background(), "ttl-key"); err != nil || !ok {
		t.Fatalf("Claim(first) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := shared.Claim(context.Background(), "ttl-key"); err != nil || ok {
		t.Fatalf("Claim(duplicate) = %v, %v; want false, nil", ok, err)
	}
	mr.FastForward(2 * time.Second)
	if ok, err := shared.Claim(context.Background(), "ttl-key"); err != nil || !ok {
		t.Fatalf("Claim(after ttl) = %v, %v; want true, nil", ok, err)
	}
}

func TestSharedDedupe_RedisFailureFallsBackToLocalDedupe(t *testing.T) {
	t.Parallel()

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 10 * time.Millisecond})
	t.Cleanup(func() { _ = rdb.Close() })
	var fallbacks atomic.Int64
	d := newRecentDedupe(16).WithShared(NewSharedDedupeStore(rdb, time.Minute), func(error) {
		fallbacks.Add(1)
	})

	if !d.Remember("redis-down") {
		t.Fatal("Remember(first) = false, want local fallback true")
	}
	if d.Remember("redis-down") {
		t.Fatal("Remember(second local) = true, want false")
	}
	if fallbacks.Load() == 0 {
		t.Fatal("fallback callback was not called")
	}
}

func TestWebhookReceiver_SharedDedupeSuppressesAcrossReceivers(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	shared := NewSharedDedupeStore(rdb, time.Minute)
	var calls atomic.Int64
	handler := HandlerFunc{TableName: "job_runs", Fn: func(context.Context, Message) error {
		calls.Add(1)
		return nil
	}}
	a := NewWebhookReceiver(nil, nil, WithWebhookSharedDedupe(shared))
	b := NewWebhookReceiver(nil, nil, WithWebhookSharedDedupe(shared))
	a.RegisterHandler(handler)
	b.RegisterHandler(handler)
	body, err := json.Marshal(Message{
		AckID:  "ack-1",
		Action: ActionUpdate,
		Record: []byte(`{"id":"run-1"}`),
		Metadata: Metadata{
			TableName:      "job_runs",
			IdempotencyKey: "idem-1",
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	for _, receiver := range []*WebhookReceiver{a, b} {
		req := httptest.NewRequest(http.MethodPost, "/cdc/webhook", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		receiver.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ServeHTTP() status = %d body = %s", rec.Code, rec.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func FuzzSharedDedupeKeys(f *testing.F) {
	for _, seed := range []string{"", "plain", "unicode-\u2603", "control-\x00-key", string(make([]byte, 2048))} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		d := newRecentDedupe(4)
		_ = d.Remember(key)
		d.Forget(key)
	})
}
