package worker

import (
	"context"
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

		payload, err := marshalRunStatusTransitionPayload(
			event.Run.ID,
			event.Run.JobID,
			event.Run.ProjectID,
			string(event.FromStatus),
			string(event.ToStatus),
			time.Now().UTC(),
		)
		if err != nil {
			slog.Error("failed to marshal event for pubsub", "run_id", event.Run.ID, "job_id", event.Run.JobID, "error", err)
			return
		}

		channel := runPubSubChannel(event.Run.ID)
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
