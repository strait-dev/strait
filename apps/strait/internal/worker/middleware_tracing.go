package worker

import (
	"context"

	"strait/internal/httputil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

// TracingMiddleware creates the top-level executor.Execute span and injects
// trace context from run metadata (W3C Traceparent/Tracestate).
func TracingMiddleware() ExecutionMiddleware {
	return func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			// Inject parent trace context from run metadata if present.
			if ec.Run.Metadata != nil {
				if tp, ok := ec.Run.Metadata["_trace_parent"]; ok && tp != "" {
					carrier := propagation.MapCarrier{
						"traceparent": tp,
					}
					if ts, ok := ec.Run.Metadata["_trace_state"]; ok && ts != "" {
						carrier["tracestate"] = ts
					}
					ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
				}
			}

			ctx, span := otel.Tracer("strait").Start(ctx, "executor.Execute")
			defer span.End()

			span.SetAttributes(
				attribute.String("run.id", ec.Run.ID),
				attribute.String("job.id", ec.Run.JobID),
				attribute.String("project.id", ec.Run.ProjectID),
				attribute.Int("run.attempt", ec.Run.Attempt),
			)
			if ec.Job != nil {
				span.SetAttributes(
					attribute.String("job.endpoint", httputil.RedactURLForLog(ec.Job.EndpointURL)),
					attribute.Int("job.version", ec.Job.Version),
				)
			}

			next(ctx, ec)
		}
	}
}
