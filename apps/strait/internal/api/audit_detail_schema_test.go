package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditActionSchemas_CompleteCoverage asserts every registered audit
// action has a corresponding schema entry. New actions must add both the
// const (audit_actions.go) and the schema (audit_schema.go).
func TestAuditActionSchemas_CompleteCoverage(t *testing.T) {
	t.Parallel()

	missing := []string{}
	for _, action := range domain.KnownAuditActions() {
		if _, ok := domain.AuditActionSchemas[action]; !ok {
			missing = append(missing, action)
		}
	}
	require.Empty(t, missing)

	// And the reverse: no stale schema entries.
	stale := []string{}
	for action := range domain.AuditActionSchemas {
		if !domain.IsKnownAuditAction(action) {
			stale = append(stale, action)
		}
	}
	require.Empty(t, stale)
}

// handlerActionPayloads maps every action to a realistic detail payload
// (matching the schema's Required keys). Used to validate that every action
// name is emittable and round-trips through the emit path with the schema
// satisfied.
var handlerActionPayloads = map[string]map[string]any{
	domain.AuditActionAPIKeyCreated:                   {"name": "CLI Key", "key_prefix": "strait_abc123"},
	domain.AuditActionAPIKeyRevoked:                   nil,
	domain.AuditActionAPIKeyRotated:                   {"new_key_id": "key-new", "grace_expires_at": "2026-04-11T12:00:00Z", "grace_period_minute": 60},
	domain.AuditActionAPIKeyListRead:                  {"count": 5},
	domain.AuditActionAuthRunTokenRejected:            {"reason": "bad_issuer", "run_id": "run-1", "issuer_present": true},
	domain.AuditActionAuditExported:                   {"from": "2026-04-01", "to": "2026-04-11", "format": "csv"},
	domain.AuditActionAuditExportCapped:               {"exported": 1000000},
	domain.AuditActionAuditListRead:                   {"count": 100},
	domain.AuditActionAuditSingleRead:                 {"target_id": "ev-1"},
	domain.AuditActionAuditChainVerified:              {"events_checked": 500, "valid": true},
	domain.AuditActionKeyRotated:                      {"previous_epoch": 1, "new_epoch": 2, "rotated_by": "actor-1"},
	domain.AuditActionRetentionTrimmed:                {"deleted_count": 42, "trimmed_before": "2026-04-01T00:00:00Z", "previous_hash": "abc123"},
	domain.AuditActionDeadletterRead:                  {"filter": "project_id=proj-1&limit=50&cursor=", "count": 3},
	domain.AuditActionDeadletterReplayed:              {"deadletter_id": "dlq-1", "new_event_id": "ev-new-1"},
	domain.AuditActionDeadletterDropped:               {"deadletter_id": "dlq-1", "reason": "operator_drop"},
	domain.AuditActionDeadletterAged:                  {"dropped_count": int64(7), "reason": "max_age_exceeded"},
	domain.AuditActionExportCapUpdated:                {"old_cap": int64(1000), "new_cap": int64(500)},
	domain.AuditActionRetentionUpdated:                {"old_days": 365, "new_days": 30},
	domain.AuditActionDeviceCodeApproved:              {"user_code": "ABCD-1234", "api_key_id": "key-1"},
	domain.AuditActionProjectCreated:                  {"name": "My Project", "org_id": "org-1"},
	domain.AuditActionProjectDeleted:                  nil,
	domain.AuditActionProjectSettingsUpdated:          {"changes": map[string]any{"default_region": "iad"}},
	domain.AuditActionEnvironmentCreated:              {"name": "prod", "slug": "prod", "parent_id": "", "variable_keys": []string{"DATABASE_URL"}},
	domain.AuditActionEnvironmentUpdated:              {"changed_fields": []string{"name"}, "name": "prod", "variable_keys": []string{"DATABASE_URL"}},
	domain.AuditActionEnvironmentDeleted:              {"name": "prod"},
	domain.AuditActionJobCreated:                      {"name": "Daily", "slug": "daily", "execution_mode": "http"},
	domain.AuditActionJobUpdated:                      {"changes": map[string]any{"name": "New"}},
	domain.AuditActionJobCloned:                       {"source_job_id": "job-1", "new_name": "Daily Copy", "new_slug": "daily-copy"},
	domain.AuditActionJobDeleted:                      nil,
	domain.AuditActionJobPaused:                       {"reason": "maintenance"},
	domain.AuditActionJobResumed:                      nil,
	domain.AuditActionJobBatchCreated:                 {"count": 10, "job_ids": []string{"job-1", "job-2"}},
	domain.AuditActionJobBatchEnabled:                 {"count": 5, "job_ids": []string{"job-1"}},
	domain.AuditActionJobBatchDisabled:                {"count": 5, "job_ids": []string{"job-1"}},
	domain.AuditActionJobTriggered:                    {"run_id": "run-1"},
	domain.AuditActionJobBulkTriggered:                {"batch_id": "batch-1", "total": 100, "created": 100},
	domain.AuditActionJobsExported:                    {"format": "ndjson", "project_id": "proj-1"},
	domain.AuditActionJobGroupCreated:                 {"name": "daily-group", "slug": "daily"},
	domain.AuditActionJobGroupUpdated:                 {"changes": map[string]any{"name": "new"}, "name": "new"},
	domain.AuditActionJobGroupDeleted:                 {"name": "g1", "slug": "g1"},
	domain.AuditActionJobGroupPausedAll:               {"name": "g1"},
	domain.AuditActionJobGroupResumedAll:              {"name": "g1"},
	domain.AuditActionJobDependencyCreated:            {"job_id": "job-1", "depends_on_job_id": "job-2", "condition": "completed"},
	domain.AuditActionJobDependencyDeleted:            {"job_id": "job-1", "depends_on_job_id": "job-2"},
	domain.AuditActionRunCancelled:                    {"job_id": "job-1", "previous_status": "executing", "children_canceled": int64(0)},
	domain.AuditActionRunReplayed:                     {"original_run_id": "run-old", "job_id": "job-1"},
	domain.AuditActionRunReplayedDeadletter:           {"original_run_id": "run-old", "job_id": "job-1"},
	domain.AuditActionRunBulkReplayedDeadletter:       {"count": 5, "project_id": "proj-1"},
	domain.AuditActionRunDebugModeSet:                 {"enabled": true},
	domain.AuditActionRunIdempotencyKeyReset:          nil,
	domain.AuditActionRunRescheduled:                  {"job_id": "job-1", "new_scheduled_at": "2026-04-11T13:00:00Z"},
	domain.AuditActionRunBulkReplayed:                 {"count": 5, "total": 5},
	domain.AuditActionRunBulkCancelled:                {"total": 5, "canceled": 5, "failed": 0},
	domain.AuditActionRunBulkCancelledAll:             {"project_id": "proj-1", "canceled": 10},
	domain.AuditActionRunPaused:                       {"job_id": "job-1"},
	domain.AuditActionRunResumed:                      {"job_id": "job-1"},
	domain.AuditActionRunRestarted:                    {"job_id": "job-1"},
	domain.AuditActionRunsExported:                    {"format": "csv", "from": "2026-04-01", "to": "2026-04-11", "project_id": "proj-1"},
	domain.AuditActionSecretCreated:                   {"secret_key": "DATABASE_URL", "environment": "prod"},
	domain.AuditActionSecretDeleted:                   {"secret_key": "DATABASE_URL", "environment": "prod"},
	domain.AuditActionSecretListRead:                  {"count": 5},
	domain.AuditActionSecretRead:                      {"secret_id": "sec-1", "name": "DATABASE_URL", "secret_key": "DATABASE_URL"},
	domain.AuditActionSSETokenCreated:                 {"expires_at": "2026-04-11T12:05:00Z", "scope_count": 3},
	domain.AuditActionRoleCreated:                     {"name": "admin", "permissions": []string{"*"}},
	domain.AuditActionRoleUpdated:                     {"changes": map[string]any{"before": map[string]any{}, "after": map[string]any{}}},
	domain.AuditActionRoleDeleted:                     nil,
	domain.AuditActionRoleSystemSeeded:                {"project_id": "proj-1"},
	domain.AuditActionPermissionGranted:               {"user_id": "user-1", "project_id": "proj-1"},
	domain.AuditActionPermissionRevoked:               {"user_id": "user-1", "project_id": "proj-1", "role_id": "role-1"},
	domain.AuditActionResourcePolicyCreated:           {"resource_type": "job", "resource_id": "job-1", "user_id": "user-1", "actions": []string{"read"}},
	domain.AuditActionResourcePolicyDeleted:           {"affected_user": "user-1"},
	domain.AuditActionTagPolicyCreated:                {"tag_key": "env", "tag_value": "prod", "resource_type": "job", "user_id": "user-1", "actions": []string{"read"}},
	domain.AuditActionTagPolicyDeleted:                nil,
	domain.AuditActionWorkflowCreated:                 {"name": "Pipeline", "slug": "pipeline", "step_count": 3},
	domain.AuditActionWorkflowUpdated:                 {"new_version": 2, "name": "Pipeline"},
	domain.AuditActionWorkflowUpdatedBreaking:         {"previous_version_id": "v1", "active_runs_on_previous_version": 5, "new_version": 2},
	domain.AuditActionWorkflowDeleted:                 {"name": "Pipeline"},
	domain.AuditActionWorkflowTriggered:               {"run_id": "wf-run-1"},
	domain.AuditActionWorkflowDryRun:                  {"step_count": 3},
	domain.AuditActionWorkflowPlanRequested:           {"step_count": 3},
	domain.AuditActionWorkflowCloned:                  {"source_workflow_id": "wf-1", "new_name": "Pipeline Copy"},
	domain.AuditActionWorkflowsExported:               {"format": "ndjson", "project_id": "proj-1"},
	domain.AuditActionWorkflowRunCancelled:            {"workflow_id": "wf-1"},
	domain.AuditActionWorkflowRunPaused:               {"workflow_id": "wf-1"},
	domain.AuditActionWorkflowRunResumed:              {"workflow_id": "wf-1"},
	domain.AuditActionWorkflowRunRetried:              {"original_run_id": "wf-run-old"},
	domain.AuditActionWorkflowRunSubtreeReplayed:      {"from_step_ref": "step-a", "reset_steps": 3},
	domain.AuditActionWorkflowRunBulkCancelled:        {"total": 5, "count": 5},
	domain.AuditActionWorkflowRunBulkReplayed:         {"total": 5, "count": 5},
	domain.AuditActionWorkflowRunCompensated:          {"workflow_id": "wf-1"},
	domain.AuditActionWorkflowStepApproved:            {"workflow_run_id": "wfr-1", "step_ref": "approve", "approver": "user-1"},
	domain.AuditActionWorkflowStepSkipped:             {"workflow_run_id": "wfr-1", "step_ref": "skip-me"},
	domain.AuditActionWorkflowStepForceCompleted:      {"workflow_run_id": "wfr-1", "step_ref": "stuck"},
	domain.AuditActionWorkflowStepRetried:             {"workflow_run_id": "wfr-1", "step_ref": "flaky"},
	domain.AuditActionWorkflowPolicyUpserted:          {"project_id": "proj-1"},
	domain.AuditActionCanaryDeploymentCreated:         {"workflow_id": "wf-1", "source_version": 1, "target_version": 2, "traffic_pct": 10},
	domain.AuditActionCanaryDeploymentUpdated:         {"workflow_id": "wf-1", "traffic_pct": 50},
	domain.AuditActionCanaryDeploymentRolledBack:      {"workflow_id": "wf-1"},
	domain.AuditActionWebhookTested:                   {"url_host": "example.com"},
	domain.AuditActionWebhookDeliveryReplayed:         {"original_delivery_id": "d-1"},
	domain.AuditActionWebhookDeliveryRetried:          {"subscription_id": "sub-1"},
	domain.AuditActionWebhookSubscriptionCreated:      {"url_host": "example.com", "event_types": []string{"run.completed"}},
	domain.AuditActionWebhookSubscriptionDeleted:      {"url_host": "example.com"},
	domain.AuditActionWebhookSubscriptionRotateSecret: {"grace_expires_at": "2026-04-12", "grace_period_minutes": 60},
	domain.AuditActionLogDrainCreated:                 {"name": "datadog", "drain_type": "datadog", "endpoint_host": "logs.datadoghq.com"},
	domain.AuditActionLogDrainUpdated:                 {"changed_fields": []string{"endpoint_url"}, "endpoint_host": "logs2.datadoghq.com"},
	domain.AuditActionLogDrainDeleted:                 nil,
	domain.AuditActionNotificationChannelCreated:      {"name": "#alerts", "channel_type": "slack"},
	domain.AuditActionNotificationChannelUpdated:      {"name": "#alerts", "channel_type": "slack"},
	domain.AuditActionNotificationChannelDeleted:      nil,
	domain.AuditActionEventSourceCreated:              {"name": "github-webhook"},
	domain.AuditActionEventSourceUpdated:              {"changed_fields": []string{"name"}},
	domain.AuditActionEventSourceDeleted:              nil,
	domain.AuditActionEventSourceSubscribed:           {"subscription_id": "sub-1", "target_type": "job", "target_id": "job-1"},
	domain.AuditActionEventSourceDispatched:           {"source_name": "github", "dispatched": 3, "payload_size": 1024},
	domain.AuditActionEventSubscriptionDeleted:        {"source_id": "src-1"},
	domain.AuditActionEventSent:                       {"event_key": "order.paid", "source_type": "workflow_step"},
	domain.AuditActionEventSentByPrefix:               {"prefix": "order.", "trigger_count": 5},
	domain.AuditActionEventTriggerCancelled:           {"event_key": "order.paid"},
	domain.AuditActionEventTriggerPurged:              {"older_than_days": 30, "deleted": 100},
	domain.AuditActionEndpointSet:                     {"job_id": "job-1", "endpoint_url_host": "example.com"},
	domain.AuditActionEndpointVerified:                {"job_id": "job-1", "endpoint_url_host": "example.com", "success": true},
	domain.AuditActionWorkerConnected:                 {"worker_id": "wkr-1", "hostname": "host-a"},
	domain.AuditActionWorkerDisconnected:              {"worker_id": "wkr-1"},
	domain.AuditActionWorkerForceDisconnected:         {"worker_id": "wkr-1", "reason": "operator_disconnect"},
	domain.AuditActionWorkerTaskRouted:                {"run_id": "run-1", "worker_id": "wkr-1", "queue": "default", "project_id": "proj-1"},
	domain.AuditActionWorkerDeleteAcked:               {"worker_id": "wkr-1"},
	domain.AuditActionWorkerDeleteTimeout:             {"worker_id": "wkr-1", "timeout_ms": float64(5000)},
	domain.AuditActionSubscriptionChanged:             {"org_id": "org-1", "plan_tier": "pro"},
	domain.AuditActionUsageThresholdReached:           {"org_id": "org-1", "plan_tier": "pro", "metric": "monthly_runs", "threshold_pct": 80, "current": int64(800), "limit": int64(1000)},
	domain.AuditActionInternalSecretBypass:            {"gate": "batch_enable_jobs.project_match", "caller": "internal_secret", "handler": "handleBatchEnableJobs"},
	domain.AuditActionDeploymentVersionCreated:        {"environment": "prod"},
	domain.AuditActionDeploymentVersionFinalized:      {"environment": "prod"},
	domain.AuditActionDeploymentVersionPromoted:       {"environment": "prod"},
	domain.AuditActionDeploymentVersionRolledBack:     {"environment": "prod"},
	domain.AuditActionSpendingLimitUpdated:            {"limit_microusd": int64(1000000000), "action": "block"},
	domain.AuditActionEmailPreferencesUpdated:         {"monthly_usage_email": true},
	domain.AuditActionUsageExported:                   {"format": "csv", "from": "2026-04-01", "to": "2026-04-11"},
	domain.AuditActionProjectBudgetUpdated:            {"budget_microusd": int64(5000000), "action": "warn"},
	domain.AuditActionAnomalyConfigUpdated:            {"warning_threshold": 1.5, "critical_threshold": 2.0},
}

// TestAuditDetailSchema_RequiredKeysPresent exercises every registered
// action through the emit path with a realistic payload and asserts the
// captured event's details contain every schema-required key.
func TestAuditDetailSchema_RequiredKeysPresent(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		captured = map[string]*domain.AuditEvent{}
	)
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured[ev.Action] = &clone
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-schema")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-schema")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	for _, action := range domain.KnownAuditActions() {
		payload := handlerActionPayloads[action]
		srv.emitAuditEvent(ctx, action, "schema_probe", "probe-1", payload)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, action := range domain.KnownAuditActions() {
		ev, ok := captured[action]
		if !ok {
			assert.Failf(t, "test failure",

				"action %q did not produce a captured event", action)
			continue
		}
		schema := domain.AuditActionSchemas[action]

		var details map[string]any
		if len(ev.Details) > 0 {
			if err := json.Unmarshal(ev.Details, &details); err != nil {
				assert.Failf(t, "test failure",

					"action %q: details unmarshal: %v", action, err)
				continue
			}
		}

		// Required keys must be present and non-zero.
		for _, key := range schema.Required {
			v, present := details[key]
			if !present {
				assert.Failf(t, "test failure",

					"action %q: missing required details key %q (have: %v)",
					action, key, keysOf(details))
				continue
			}
			assert.False(
				t, isZero(v))
		}

		// Forbidden keys must be absent.
		for _, key := range domain.ForbiddenKeysFor(action) {
			if _, present := details[key]; present {
				assert.Failf(t, "test failure",

					"action %q: forbidden details key %q is present (defense-in-depth violation)",
					action, key)
			}
		}
	}
}

// keysOf returns the sorted keys of a map for human-friendly error messages.
func keysOf(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// isZero reports whether v is a go zero value for its type — nil, "",
// 0, false, or an empty slice/map. Used to reject empty required keys.
func isZero(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case bool:
		return false // a present bool is non-zero by presence alone (false is meaningful)
	case float64:
		return x == 0
	case int:
		return x == 0
	case int64:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}
