package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResilientPublisher_PublishBatch_DegradeAfterThreshold(t *testing.T) {
	t.Parallel()
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("redis down")
		},
	}

	rp := NewResilientPublisher(mock, nil, 2)
	require.True(t,
		rp.IsHealthy())

	// First batch failure.
	_ = rp.PublishBatch(context.Background(), []PubSubMessage{{Channel: "c", Data: []byte("d")}})
	assert.True(t, rp.
		IsHealthy())

	// Second failure should degrade.
	_ = rp.PublishBatch(context.Background(), []PubSubMessage{{Channel: "c", Data: []byte("d")}})
	assert.False(t,
		rp.IsHealthy())

}

func TestResilientPublisher_PublishBatch_RecoverAfterSuccess(t *testing.T) {
	t.Parallel()
	var failCount atomic.Int32
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			if failCount.Add(1) <= 3 {
				return errors.New("temporary failure")
			}
			return nil
		},
	}

	rp := NewResilientPublisher(mock, nil, 2)

	// Fail 3 times to degrade.
	for range 3 {
		_ = rp.PublishBatch(context.Background(), []PubSubMessage{{Channel: "c", Data: []byte("d")}})
	}
	require.False(t,
		rp.IsHealthy())

	// Successful batch should recover.
	_ = rp.PublishBatch(context.Background(), []PubSubMessage{{Channel: "c", Data: []byte("d")}})
	assert.True(t, rp.
		IsHealthy())

}

func TestResilientPublisher_PublishBatch_SingleMessage_Optimization(t *testing.T) {
	t.Parallel()
	var publishCalls atomic.Int32
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			publishCalls.Add(1)
			return nil
		},
	}

	rp := NewResilientPublisher(mock, nil, 3)
	// Single message batch should still work.
	_ = rp.PublishBatch(context.Background(), []PubSubMessage{
		{Channel: "ch", Data: []byte("single")},
	})
	assert.True(t, rp.
		IsHealthy())

}

func TestResilientPublisher_PublishBatch_LargeBatch(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var received int

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			mu.Lock()
			received++
			mu.Unlock()
			return nil
		},
	}

	rp := NewResilientPublisher(mock, nil, 3)
	msgs := make([]PubSubMessage, 500)
	for i := range msgs {
		msgs[i] = PubSubMessage{
			Channel: fmt.Sprintf("ch:%d", i),
			Data:    fmt.Appendf(nil, "data-%d", i),
		}
	}

	err := rp.PublishBatch(context.Background(), msgs)
	require.NoError(
		t, err)

}

func TestResilientPublisher_PublishBatch_ConcurrentBatches(t *testing.T) {
	t.Parallel()
	var totalCalls atomic.Int64
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			totalCalls.Add(1)
			return nil
		},
	}

	rp := NewResilientPublisher(mock, nil, 3)

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			msgs := []PubSubMessage{
				{Channel: fmt.Sprintf("ch:%d:a", i), Data: []byte("a")},
				{Channel: fmt.Sprintf("ch:%d:b", i), Data: []byte("b")},
			}
			_ = rp.PublishBatch(context.Background(), msgs)
		})
	}
	wg.Wait()
	assert.True(t, rp.
		IsHealthy())

}

func TestPubSubMessage_EmptyData(t *testing.T) {
	t.Parallel()
	msg := PubSubMessage{Channel: "ch", Data: nil}
	assert.Nil(t, msg.Data)

	msg2 := PubSubMessage{Channel: "ch", Data: []byte{}}
	assert.Len(t, msg2.
		Data, 0,
	)

}

func TestPubSubMessage_EmptyChannel(t *testing.T) {
	t.Parallel()
	msg := PubSubMessage{Channel: "", Data: []byte("data")}
	assert.Equal(t,
		"", msg.Channel,
	)

}
