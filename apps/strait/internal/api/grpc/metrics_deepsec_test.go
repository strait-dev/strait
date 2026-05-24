package grpc

import "testing"

func TestDeepSecWorkerQueueMetricKindDoesNotExposeQueueNames(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                    "default",
		"default":             "default",
		"billing-prod":        "custom",
		"tenant-secret-queue": "custom",
	}
	for queue, want := range cases {
		if got := workerQueueMetricKind(queue); got != want {
			t.Fatalf("workerQueueMetricKind(%q) = %q, want %q", queue, got, want)
		}
	}
}
