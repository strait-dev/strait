package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRedisPublisher_PublishBatch_Empty(t *testing.T) {
	t.Parallel()
	// PublishBatch with empty slice should be a no-op.
	rp := &RedisPublisher{client: nil}
	err := rp.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("PublishBatch(nil) = %v, want nil", err)
	}

	err = rp.PublishBatch(context.Background(), []PubSubMessage{})
	if err != nil {
		t.Fatalf("PublishBatch([]) = %v, want nil", err)
	}
}

func TestResilientPublisher_PublishBatch_FailOpen(t *testing.T) {
	t.Parallel()
	var calls int
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			calls++
			return errors.New("redis down")
		},
	}

	rp := NewResilientPublisher(mock, nil, 3)
	err := rp.PublishBatch(context.Background(), []PubSubMessage{
		{Channel: "ch1", Data: []byte("msg1")},
		{Channel: "ch2", Data: []byte("msg2")},
	})

	// Resilient publisher swallows errors (fail-open).
	if err != nil {
		t.Fatalf("PublishBatch() = %v, want nil (fail-open)", err)
	}
}

func TestResilientPublisher_PublishBatch_NilPublisher(t *testing.T) {
	t.Parallel()
	rp := NewResilientPublisher(nil, nil, 3)
	err := rp.PublishBatch(context.Background(), []PubSubMessage{
		{Channel: "ch1", Data: []byte("msg1")},
	})
	if err != nil {
		t.Fatalf("PublishBatch(nil publisher) = %v, want nil", err)
	}
}

func TestResilientPublisher_PublishBatch_Success(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var published []PubSubMessage

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, ch string, data []byte) error {
			mu.Lock()
			published = append(published, PubSubMessage{Channel: ch, Data: data})
			mu.Unlock()
			return nil
		},
	}

	rp := NewResilientPublisher(mock, nil, 3)
	msgs := []PubSubMessage{
		{Channel: "ch1", Data: []byte("msg1")},
		{Channel: "ch2", Data: []byte("msg2")},
		{Channel: "ch3", Data: []byte("msg3")},
	}

	err := rp.PublishBatch(context.Background(), msgs)
	if err != nil {
		t.Fatalf("PublishBatch() = %v, want nil", err)
	}
	if !rp.IsHealthy() {
		t.Error("publisher should be healthy after successful batch")
	}
}

func TestPubSubMessage_Fields(t *testing.T) {
	t.Parallel()
	msg := PubSubMessage{Channel: "test:channel", Data: []byte("payload")}
	if msg.Channel != "test:channel" {
		t.Errorf("Channel = %q, want %q", msg.Channel, "test:channel")
	}
	if string(msg.Data) != "payload" {
		t.Errorf("Data = %q, want %q", msg.Data, "payload")
	}
}
