package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
)

func addWorkerRunBreadcrumb(ctx context.Context, category, message string, run *domain.JobRun, job *domain.Job, data map[string]any) {
	if run == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["run_id"] = run.ID
	data["job_id"] = run.JobID
	data["project_id"] = run.ProjectID
	data["attempt"] = run.Attempt
	data["status"] = string(run.Status)
	data["execution_mode"] = string(run.ExecutionMode)
	if job != nil {
		data["job_version"] = job.Version
		data["environment_id"] = job.EnvironmentID
	}
	telemetry.AddSentryBreadcrumb(ctx, category, message, data)
}

func (e *Executor) applyWorkerSentryScope(scope *sentry.Scope, run *domain.JobRun, data map[string]any) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemWorker,
		Mode:      e.mode,
		Region:    e.defaultRegion,
		Version:   e.version,
	})
	if run != nil {
		telemetry.SetSentryTag(scope, telemetry.TagRunID, run.ID)
		telemetry.SetSentryTag(scope, telemetry.TagJobID, run.JobID)
		telemetry.SetSentryTag(scope, telemetry.TagProjectID, run.ProjectID)
		telemetry.SetSentryTag(scope, telemetry.TagAttempt, strconv.Itoa(run.Attempt))
		if run.CreatedBy != "" {
			actorType := workerActorType(run)
			telemetry.SetSentryTag(scope, telemetry.TagActorID, run.CreatedBy)
			telemetry.SetSentryTag(scope, telemetry.TagActorType, actorType)
			scope.SetUser(sentry.User{
				ID: run.CreatedBy,
				Data: map[string]string{
					"actor_type": actorType,
					"project_id": run.ProjectID,
				},
			})
		}
		requestContext := sentry.Context{
			"created_by":   run.CreatedBy,
			"triggered_by": run.TriggeredBy,
		}
		if requestID := run.Metadata[domain.RunMetadataSentryRequestID]; requestID != "" {
			telemetry.SetSentryTag(scope, telemetry.TagRequestID, requestID)
			requestContext["request_id"] = requestID
		}
		route := run.Metadata[domain.RunMetadataSentryRoute]
		if route == "" {
			route = "worker.dispatch"
		}
		telemetry.SetSentryTag(scope, telemetry.TagRoute, route)
		requestContext["route"] = route
		if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
			requestContext["actor_type"] = actorType
		}
		scope.SetContext("dispatch.request", requestContext)
		scope.SetContext("run", sentry.Context{
			"run_id":         run.ID,
			"job_id":         run.JobID,
			"project_id":     run.ProjectID,
			"attempt":        run.Attempt,
			"priority":       run.Priority,
			"execution_mode": string(run.ExecutionMode),
			"status":         string(run.Status),
		})
	}
	for key, val := range data {
		if tag, ok := telemetry.SentryTagFromString(key); ok {
			telemetry.SetSentryTag(scope, tag, fmt.Sprintf("%v", val))
		}
	}
}

func workerActorType(run *domain.JobRun) string {
	if run == nil {
		return ""
	}
	if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
		return actorType
	}
	switch {
	case strings.HasPrefix(run.CreatedBy, "apikey:"):
		return "api_key"
	case strings.HasPrefix(run.CreatedBy, "run:"):
		return "run_token"
	case strings.HasPrefix(run.CreatedBy, "sse:"):
		return "sse_token"
	case run.CreatedBy != "":
		return "user"
	default:
		return ""
	}
}
