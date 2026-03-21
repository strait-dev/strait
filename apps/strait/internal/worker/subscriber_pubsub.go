package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/pubsub"

	"go.opentelemetry.io/otel/metric"
)

// PubSubSubscriber publishes run lifecycle events to Redis pub/sub channels.
func PubSubSubscriber(pub pubsub.Publisher, errorCounter ...metric.Int64Counter) RunEventSubscriber {
	var errCounter metric.Int64Counter
	if len(errorCounter) > 0 {
		errCounter = errorCounter[0]
	}
	return func(ctx context.Context, event RunLifecycleEvent) {
		if event.Run == nil {
			return
		}

		data := map[string]any{
			"type":       "status_change",
			"run_id":     event.Run.ID,
			"job_id":     event.Run.JobID,
			"project_id": event.Run.ProjectID,
			"from":       string(event.FromStatus),
			"to":         string(event.ToStatus),
			"timestamp":  time.Now().UTC(),
		}

		payload, err := json.Marshal(data)
		if err != nil {
			slog.Error("failed to marshal event for pubsub", "run_id", event.Run.ID, "job_id", event.Run.JobID, "error", err)
			return
		}

		channel := fmt.Sprintf("run:%s", event.Run.ID)
		if err := pub.Publish(ctx, channel, payload); err != nil {
			slog.Error("failed to publish event",
				"run_id", event.Run.ID,
				"job_id", event.Run.JobID,
				"project_id", event.Run.ProjectID,
				"channel", channel,
				"error", err,
			)
			if errCounter != nil {
				errCounter.Add(ctx, 1)
			}
		}
	}
}
