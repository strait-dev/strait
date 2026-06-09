package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var grpcControlChannelSink string

func TestGRPCPubSubChannels(t *testing.T) {
	t.Parallel()

	require.Equal(t, "run:run-1", grpcRunPubSubChannel("run-1"))
	require.Equal(t, "worker:log:run-1", grpcWorkerLogChannel("run-1"))
	require.Equal(t, "worker:disconnect:proj-1:worker-1", workerDisconnectChannel("proj-1", "worker-1"))
	require.Equal(t, "worker:disconnect_ack:proj-1:worker-1", workerDisconnectAckChannel("proj-1", "worker-1"))
	require.Equal(t, "apikey:revoked:key-1", apiKeyRevokedChannel("key-1"))
	require.Equal(t, "apikey:expires:key-1", apiKeyExpiresChannel("key-1"))
}

func BenchmarkGRPCPubSubChannels(b *testing.B) {
	runID := "run_0123456789abcdef"

	b.Run("run", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			channel := grpcRunPubSubChannel(runID)
			if channel == "" {
				b.Fatal("grpcRunPubSubChannel() returned empty channel")
			}
		}
	})

	b.Run("worker_log", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			channel := grpcWorkerLogChannel(runID)
			if channel == "" {
				b.Fatal("grpcWorkerLogChannel() returned empty channel")
			}
		}
	})
}

func BenchmarkGRPCControlChannels(b *testing.B) {
	projectID := "proj_0123456789abcdef"
	workerID := "worker_0123456789abcdef"
	apiKeyID := "key_0123456789abcdef"

	b.Run("WorkerDisconnect", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			grpcControlChannelSink = workerDisconnectChannel(projectID, workerID)
		}
	})

	b.Run("WorkerDisconnectAck", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			grpcControlChannelSink = workerDisconnectAckChannel(projectID, workerID)
		}
	})

	b.Run("APIKeyRevoked", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			grpcControlChannelSink = apiKeyRevokedChannel(apiKeyID)
		}
	})

	b.Run("APIKeyExpires", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			grpcControlChannelSink = apiKeyExpiresChannel(apiKeyID)
		}
	})
}
