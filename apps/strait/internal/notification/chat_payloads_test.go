package notification

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatPayloadMarshalers(t *testing.T) {
	t.Parallel()

	slackPayload, err := marshalSlackPayload("run.completed", []byte(`{"run_id":"r-1"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"text":"[run.completed] {\"run_id\":\"r-1\"}"}`, string(slackPayload))

	discordPayload, err := marshalDiscordPayload("run.completed", []byte(`{"run_id":"r-1"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"content":"[run.completed] {\"run_id\":\"r-1\"}"}`, string(discordPayload))
}

func BenchmarkChatPayloadMarshalers(b *testing.B) {
	payload := []byte(`{"run_id":"r-1","job_id":"j-1","status":"completed","result":{"ok":true}}`)

	b.Run("slack", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			body, err := marshalSlackPayload("run.completed", payload)
			if err != nil {
				b.Fatal(err)
			}
			if len(body) == 0 {
				b.Fatal("empty body")
			}
		}
	})

	b.Run("discord", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			body, err := marshalDiscordPayload("run.completed", payload)
			if err != nil {
				b.Fatal(err)
			}
			if len(body) == 0 {
				b.Fatal("empty body")
			}
		}
	})
}
