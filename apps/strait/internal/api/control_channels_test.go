package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var apiControlChannelSink string

func TestControlChannels(t *testing.T) {
	t.Parallel()

	require.Equal(t, "worker:disconnect:proj-1:worker-1", workerDisconnectChannel("proj-1", "worker-1"))
	require.Equal(t, "worker:disconnect_ack:proj-1:worker-1", workerDisconnectAckChannel("proj-1", "worker-1"))
	require.Equal(t, "apikey:revoked:key-1", apiKeyRevokedChannel("key-1"))
	require.Equal(t, "apikey:expires:key-1", apiKeyExpiresChannel("key-1"))
}

func BenchmarkControlChannels(b *testing.B) {
	projectID := "proj_0123456789abcdef"
	workerID := "worker_0123456789abcdef"
	apiKeyID := "key_0123456789abcdef"

	b.Run("WorkerDisconnect", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			apiControlChannelSink = workerDisconnectChannel(projectID, workerID)
		}
	})

	b.Run("WorkerDisconnectAck", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			apiControlChannelSink = workerDisconnectAckChannel(projectID, workerID)
		}
	})

	b.Run("APIKeyRevoked", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			apiControlChannelSink = apiKeyRevokedChannel(apiKeyID)
		}
	})

	b.Run("APIKeyExpires", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			apiControlChannelSink = apiKeyExpiresChannel(apiKeyID)
		}
	})
}
