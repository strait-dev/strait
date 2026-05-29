package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"strait/internal/domain"
)

const (
	pgQueHTTPRouteKey = "http"
	pgQueQueuePrefix  = "stq_"
)

func pgQueQueueName(routeKey string) string {
	sum := sha256.Sum256([]byte(routeKey))
	return pgQueQueuePrefix + hex.EncodeToString(sum[:])[:32]
}

func pgQueRouteKeyForRun(run *domain.JobRun) string {
	if run != nil && run.ExecutionMode == domain.ExecutionModeWorker {
		return pgQueWorkerRouteKey(run.ProjectID, runQueueName(run.QueueName), "")
	}
	return pgQueHTTPRouteKey
}

func pgQueWorkerRouteKey(projectID, queueName, environmentID string) string {
	return strings.Join([]string{
		"worker",
		projectID,
		runQueueName(queueName),
		environmentID,
	}, ":")
}
