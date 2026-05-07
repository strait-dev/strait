package grpc

import (
	"testing"
)

// TestBounds_Constants pins the worker-plane resource bounds. These caps are
// the only thing standing between a malicious or buggy worker and unbounded
// memory / DB / pubsub-channel growth on the server. Any change to a cap
// should be deliberate and reviewed.
func TestBounds_Constants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"maxWorkerIDLen", maxWorkerIDLen, 128},
		{"maxQueuesPerWorker", maxQueuesPerWorker, 64},
		{"maxQueueNameBytes", maxQueueNameBytes, 128},
		{"maxJobSlugsPerWorker", maxJobSlugsPerWorker, 256},
		{"maxJobSlugBytes", maxJobSlugBytes, 128},
		{"maxInFlightTasks", maxInFlightTasks, 256},
		{"maxLogMessageBytes", maxLogMessageBytes, 4096},
		{"maxLogLevelBytes", maxLogLevelBytes, 32},
		{"maxRunIDLen", maxRunIDLen, 128},
		{"maxErrorMsgBytes", maxErrorMsgBytes, 8192},
		{"maxSlotsPerWorker", maxSlotsPerWorker, 1024},
		{"maxHostnameBytes", maxHostnameBytes, 255},
		{"maxSDKVersionBytes", maxSDKVersionBytes, 64},
		{"maxSDKLanguageBytes", maxSDKLanguageBytes, 32},
		{"maxNameBytes", maxNameBytes, 128},
		{"maxRegistrationMetadataEntries", maxRegistrationMetadataEntries, 64},
		{"maxRegistrationMetadataKeyBytes", maxRegistrationMetadataKeyBytes, 64},
		{"maxRegistrationMetadataValueBytes", maxRegistrationMetadataValueBytes, 512},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}
