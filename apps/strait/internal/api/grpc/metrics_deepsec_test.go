package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSecWorkerQueueMetricKindDoesNotExposeQueueNames(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                    "default",
		"default":             "default",
		"billing-prod":        "custom",
		"tenant-secret-queue": "custom",
	}
	for queue, want := range cases {
		require.Equal(t, want, workerQueueMetricKind(queue))
	}
}
