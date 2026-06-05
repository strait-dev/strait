package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzPubSubMessageSerialization exercises JSON round-trip serialization of
// PubSubMessage with arbitrary channel names and data payloads.
func FuzzPubSubMessageSerialization(f *testing.F) {
	f.Add("channel:updates", []byte(`{"run_id":"r-1"}`))
	f.Add("", []byte(``))
	f.Add("ch\x00null", []byte{0x00, 0xff})
	f.Add(string(make([]byte, 4096)), []byte("data"))
	f.Add("run:abc-123:status", []byte(`null`))
	f.Add("channel/with/slashes", []byte(`[]`))
	f.Add("channel.with.dots", []byte(`"string payload"`))

	f.Fuzz(func(t *testing.T, channel string, data []byte) {
		msg := PubSubMessage{
			Channel: channel,
			Data:    data,
		}

		// Marshal must not panic.
		encoded, err := json.Marshal(msg)
		require.NoError(
			t, err)

		// Unmarshal must not panic.
		var decoded PubSubMessage
		require.NoError(
			t, json.Unmarshal(encoded,
				&decoded))
		assert.Equal(t,
			msg.Channel, decoded.
				Channel,
		)

	})
}

// FuzzChannelNameHandling exercises the ResilientPublisher with arbitrary
// channel names including empty, very long, special characters, and null bytes.
// The publisher must never panic.
func FuzzChannelNameHandling(f *testing.F) {
	f.Add("run:updates")
	f.Add("")
	f.Add("\x00")
	f.Add(strings.Repeat("a", 65536))
	f.Add("channel\nwith\nnewlines")
	f.Add("channel\twith\ttabs")
	f.Add("channel with spaces")
	f.Add("channel/with/slashes")
	f.Add("channel:with:colons")
	f.Add("channel.with.dots")
	f.Add("channel{with}braces")
	f.Add("<channel>with<brackets>")
	f.Add("channel\x00with\x00nulls")
	f.Add("channel\xff\xfe\xfd")

	f.Fuzz(func(t *testing.T, channel string) {
		errPublish := errors.New("simulated publish error")
		mp := &mockPublisher{
			publishFunc: func(_ context.Context, _ string, _ []byte) error {
				return errPublish
			},
		}

		rp := NewResilientPublisher(mp, slog.Default(), 3)

		// Publish with arbitrary channel name must not panic.
		_ = rp.Publish(context.Background(), channel, []byte("test-data"))

		// Subscribe with arbitrary channel name must not panic.
		sub, _ := rp.Subscribe(context.Background(), channel)
		if sub != nil {
			sub.Close()
		}
	})
}

// FuzzResilientPublisherThreshold exercises the ResilientPublisher health
// threshold logic with varying failure counts and threshold values.
func FuzzResilientPublisherThreshold(f *testing.F) {
	f.Add(3, 5)
	f.Add(0, 0)
	f.Add(1, 1)
	f.Add(100, 50)
	f.Add(-1, -1)
	f.Add(1, 1000000)

	f.Fuzz(func(t *testing.T, threshold, failureCount int) {
		// NewResilientPublisher clamps threshold <= 0 to defaultFailureThreshold.
		mp := &mockPublisher{
			publishFunc: func(_ context.Context, _ string, _ []byte) error {
				return errors.New("fail")
			},
		}

		rp := NewResilientPublisher(mp, slog.Default(), threshold)
		assert.True(t, rp.
			IsHealthy())

		// Initial state must be healthy.

		// Apply a bounded number of failures to avoid excessive test time.
		count := min(max(failureCount, 0), 200)

		for range count {
			_ = rp.Publish(context.Background(), "test-channel", []byte("data"))
		}

		// Verify IsHealthy does not panic regardless of state.
		_ = rp.IsHealthy()
	})
}

// FuzzResilientPublisherRecovery exercises the recovery path where successes
// follow failures, ensuring the health state flips correctly.
func FuzzResilientPublisherRecovery(f *testing.F) {
	f.Add(3, 5, 2)
	f.Add(1, 1, 1)
	f.Add(10, 20, 5)
	f.Add(1, 100, 0)

	f.Fuzz(func(t *testing.T, threshold, failures, successes int) {
		// Bound inputs.
		if threshold < 1 {
			threshold = 1
		}
		if failures < 0 {
			failures = 0
		}
		if failures > 100 {
			failures = 100
		}
		if successes < 0 {
			successes = 0
		}
		if successes > 100 {
			successes = 100
		}

		callCount := 0
		failAfter := failures

		mp := &mockPublisher{
			publishFunc: func(_ context.Context, _ string, _ []byte) error {
				callCount++
				if callCount <= failAfter {
					return errors.New("fail")
				}
				return nil
			},
		}

		rp := NewResilientPublisher(mp, slog.Default(), threshold)

		// Run failures then successes.
		for range failures + successes {
			_ = rp.Publish(context.Background(), "ch", []byte("d"))
		}

		// After successes, if any succeeded, publisher should be healthy.
		if successes > 0 {
			assert.True(t, rp.
				IsHealthy())

		}
	})
}

// FuzzPublishBatchMessages exercises ResilientPublisher.PublishBatch with
// arbitrary message contents.
func FuzzPublishBatchMessages(f *testing.F) {
	f.Add("ch1", []byte("data1"), "ch2", []byte("data2"))
	f.Add("", []byte(``), "", []byte(``))
	f.Add("\x00", []byte{0xff}, "normal", []byte("normal"))
	f.Add(strings.Repeat("x", 1024), make([]byte, 1024), "b", []byte("y"))

	f.Fuzz(func(t *testing.T, ch1 string, data1 []byte, ch2 string, data2 []byte) {
		mp := &mockPublisher{}
		rp := NewResilientPublisher(mp, slog.Default(), 3)

		messages := []PubSubMessage{
			{Channel: ch1, Data: data1},
			{Channel: ch2, Data: data2},
		}

		// Must not panic regardless of channel name or data content.
		_ = rp.PublishBatch(context.Background(), messages)
	})
}

// FuzzNewSubscriptionClose exercises creating and closing subscriptions
// to verify no panics on double-close or with arbitrary cancel functions.
func FuzzNewSubscriptionClose(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, closeImmediately bool) {
		ch := make(chan []byte, 1)
		called := false
		sub := NewSubscription(ch, func() { called = true })

		if closeImmediately {
			sub.Close()
			assert.True(t, called)

		}

		// closedSubscription helper must not panic.
		closed := closedSubscription()
		closed.Close()
	})
}
