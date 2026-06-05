package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisPublisher_PublishBatch_Empty(t *testing.T) {
	t.Parallel()
	// PublishBatch with empty slice should be a no-op.
	rp := &RedisPublisher{client: nil}
	err := rp.PublishBatch(context.Background(), nil)
	require.NoError(
		t, err)

	err = rp.PublishBatch(context.Background(), []PubSubMessage{})
	require.NoError(
		t, err)

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
	require.NoError(
		t, err)

	// Resilient publisher swallows errors (fail-open).

}

func TestResilientPublisher_PublishBatch_NilPublisher(t *testing.T) {
	t.Parallel()
	rp := NewResilientPublisher(nil, nil, 3)
	err := rp.PublishBatch(context.Background(), []PubSubMessage{
		{Channel: "ch1", Data: []byte("msg1")},
	})
	require.NoError(
		t, err)

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
	require.NoError(
		t, err)
	assert.True(t, rp.
		IsHealthy())

}

func TestPubSubMessage_Fields(t *testing.T) {
	t.Parallel()
	msg := PubSubMessage{Channel: "test:channel", Data: []byte("payload")}
	assert.Equal(t,
		"test:channel",
		msg.
			Channel)
	assert.Equal(t,
		"payload",
		string(
			msg.Data))

}
