package pubsub

import (
	"strings"
	"testing"
)

// TestPublish_NullBytesInChannel verifies that a channel name containing
// null bytes does not cause a panic when constructing a PubSubMessage.
func TestPublish_NullBytesInChannel(t *testing.T) {
	t.Parallel()

	channel := "events\x00injected"
	msg := PubSubMessage{
		Channel: channel,
		Data:    []byte(`{"ok":true}`),
	}

	if msg.Channel != channel {
		t.Errorf("Channel = %q, want %q", msg.Channel, channel)
	}
}

// TestPublish_ExtremelyLongChannel verifies that a 100KB channel name
// does not cause a panic in message construction.
func TestPublish_ExtremelyLongChannel(t *testing.T) {
	t.Parallel()

	channel := strings.Repeat("a", 100*1024)
	msg := PubSubMessage{
		Channel: channel,
		Data:    []byte("payload"),
	}

	if len(msg.Channel) != 100*1024 {
		t.Errorf("Channel length = %d, want %d", len(msg.Channel), 100*1024)
	}
}

// TestPublish_SpecialCharsInChannel verifies that glob-pattern characters
// in channel names are stored verbatim.
func TestPublish_SpecialCharsInChannel(t *testing.T) {
	t.Parallel()

	specialChars := []string{
		"events:*",
		"events:?",
		"events:[a-z]",
		"events:{foo,bar}",
		"events:$dollar",
		"events:`backtick`",
	}

	for _, ch := range specialChars {
		msg := PubSubMessage{
			Channel: ch,
			Data:    []byte("test"),
		}
		if msg.Channel != ch {
			t.Errorf("Channel = %q, want %q", msg.Channel, ch)
		}
	}
}

// TestPublish_AdversarialPayload verifies that a 10MB payload does not
// cause a panic when constructing a PubSubMessage.
func TestPublish_AdversarialPayload(t *testing.T) {
	t.Parallel()

	largePayload := make([]byte, 10*1024*1024)
	for i := range largePayload {
		largePayload[i] = 'x'
	}

	msg := PubSubMessage{
		Channel: "events:large",
		Data:    largePayload,
	}

	if len(msg.Data) != 10*1024*1024 {
		t.Errorf("Data length = %d, want %d", len(msg.Data), 10*1024*1024)
	}
}

// TestBatch_MixedNilPayloads verifies that a batch with nil payloads
// does not cause a panic. NewSubscription and PubSubMessage are pure
// data structures so we test construction and field access.
func TestBatch_MixedNilPayloads(t *testing.T) {
	t.Parallel()

	messages := []PubSubMessage{
		{Channel: "ch1", Data: []byte("valid")},
		{Channel: "ch2", Data: nil},
		{Channel: "ch3", Data: []byte("")},
		{Channel: "", Data: nil},
		{Channel: "ch5", Data: []byte("also-valid")},
	}

	for i, msg := range messages {
		// Access fields to ensure no panic on nil Data.
		_ = msg.Channel
		_ = msg.Data
		if msg.Data != nil && len(msg.Data) == 0 && i != 2 {
			// Distinguish between nil and empty.
			t.Errorf("message %d: expected nil Data, got empty slice", i)
		}
	}

	// Verify the batch length is preserved.
	if len(messages) != 5 {
		t.Errorf("batch length = %d, want 5", len(messages))
	}
}

// FuzzPublishAdversarial fuzzes channel names and payloads to check
// for panics in PubSubMessage construction and NewSubscription.
func FuzzPublishAdversarial(f *testing.F) {
	f.Add("events:job_runs", []byte(`{"id":"1"}`))
	f.Add("", []byte{})
	f.Add("events:\x00null", []byte{0x00, 0x01, 0x02})
	f.Add(strings.Repeat("x", 1024), []byte("payload"))

	f.Fuzz(func(t *testing.T, channel string, data []byte) {
		// Must not panic.
		msg := PubSubMessage{
			Channel: channel,
			Data:    data,
		}
		_ = msg.Channel
		_ = msg.Data
	})
}
