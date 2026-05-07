package domain

// Audit action names. Every call to emitAuditEvent / emitAuditEventAsync in
// the api package MUST reference one of these constants — the AST-based
// TestAuditActionRegistryCoverage test enforces this.
//
// Adding a new action:
//   1. Add a const here in the appropriate section
//   2. Add a short description in audit_schema.go (AuditActionSchemas)
//   3. Use the const from the handler
//
// Renaming an action is a breaking change for downstream consumers — prefer
// adding a new action over renaming.
//
// Naming convention: <resource_type>.<past_tense_verb>. The few historical
// oddities (job.delete, api_key.revoke, api_key.rotate) are kept verbatim so
// existing audit rows continue to match filters in downstream tooling.

//nolint:gosec // G101 false positives: these are audit action name constants, not hardcoded credentials.
const (
	// API keys.
	AuditActionAPIKeyCreated  = "api_key.created"
	AuditActionAPIKeyRevoked  = "api_key.revoke"
	AuditActionAPIKeyRotated  = "api_key.rotate"
	AuditActionAPIKeyListRead = "api_key.list_read"

	// SDK run-token auth.
	AuditActionAuthRunTokenRejected = "auth.run_token.rejected"

	// Audit log self-audit + read access (SOC 2 requires audit of audit reads).
	AuditActionAuditExported      = "audit.exported"
	AuditActionAuditExportCapped  = "audit.export_capped"
	AuditActionAuditListRead      = "audit.list_read"
	AuditActionAuditSingleRead    = "audit.single_read"
	AuditActionAuditChainVerified = "audit.chain_verified"
	AuditActionKeyRotated         = "audit.key_rotated"
	AuditActionRetentionTrimmed   = "audit.retention_trimmed"
	AuditActionDeadletterRead     = "audit.deadletter_read"
	AuditActionDeadletterReplayed = "audit.deadletter_replayed"
	AuditActionDeadletterDropped  = "audit.deadletter_dropped"
	AuditActionDeadletterAged     = "audit.deadletter_aged"
	AuditActionExportCapUpdated   = "audit.export_cap_updated"
	AuditActionRetentionUpdated   = "audit.retention_updated"

	// CLI / device code.
	AuditActionDeviceCodeApproved = "device_code.approved"

	// Projects.
	AuditActionProjectCreated         = "project.created"
	AuditActionProjectDeleted         = "project.deleted"
	AuditActionProjectSettingsUpdated = "project_settings.updated"

	// Environments.
	AuditActionEnvironmentCreated = "environment.created"
	AuditActionEnvironmentUpdated = "environment.updated"
	AuditActionEnvironmentDeleted = "environment.deleted"

	// Jobs (lifecycle).
	AuditActionJobCreated       = "job.created"
	AuditActionJobUpdated       = "job.updated"
	AuditActionJobCloned        = "job.cloned"
	AuditActionJobDeleted       = "job.delete"
	AuditActionJobPaused        = "job.paused"
	AuditActionJobResumed       = "job.resumed"
	AuditActionJobBatchCreated  = "job.batch_created"
	AuditActionJobBatchEnabled  = "job.batch_enabled"
	AuditActionJobBatchDisabled = "job.batch_disabled"
	AuditActionJobTriggered     = "job.triggered"
	AuditActionJobBulkTriggered = "job.bulk_triggered"
	AuditActionJobsExported     = "jobs.exported"

	// Job groups.
	AuditActionJobGroupCreated    = "job_group.created"
	AuditActionJobGroupUpdated    = "job_group.updated"
	AuditActionJobGroupDeleted    = "job_group.deleted"
	AuditActionJobGroupPausedAll  = "job_group.paused_all"
	AuditActionJobGroupResumedAll = "job_group.resumed_all"

	// Job dependencies.
	AuditActionJobDependencyCreated = "job_dependency.created"
	AuditActionJobDependencyDeleted = "job_dependency.deleted"

	// Runs (lifecycle).
	AuditActionRunCancelled              = "run.cancelled"
	AuditActionRunReplayed               = "run.replayed"
	AuditActionRunReplayedDeadletter     = "run.replayed_deadletter"
	AuditActionRunBulkReplayedDeadletter = "run.bulk_replayed_deadletter"
	AuditActionRunDebugModeSet           = "run.debug_mode_set"
	AuditActionRunIdempotencyKeyReset    = "run.idempotency_key_reset"
	AuditActionRunRescheduled            = "run.rescheduled"
	AuditActionRunBulkReplayed           = "run.bulk_replayed"
	AuditActionRunBulkCancelled          = "run.bulk_cancelled"
	AuditActionRunBulkCancelledAll       = "run.bulk_cancelled_all"
	AuditActionRunPaused                 = "run.paused"
	AuditActionRunResumed                = "run.resumed"
	AuditActionRunRestarted              = "run.restarted"
	AuditActionRunsExported              = "runs.exported"

	// Secrets.
	AuditActionSecretCreated  = "secret.created"
	AuditActionSecretDeleted  = "secret.deleted"
	AuditActionSecretListRead = "secret.list_read"
	AuditActionSecretRead     = "secret.read"

	// SSE token.
	AuditActionSSETokenCreated = "sse_token.created"

	// RBAC.
	AuditActionRoleCreated           = "role.created"
	AuditActionRoleUpdated           = "role.updated"
	AuditActionRoleDeleted           = "role.deleted"
	AuditActionRoleSystemSeeded      = "role.system_seeded"
	AuditActionPermissionGranted     = "permission.granted"
	AuditActionPermissionRevoked     = "permission.revoked"
	AuditActionResourcePolicyCreated = "resource_policy.created"
	AuditActionResourcePolicyDeleted = "resource_policy.deleted"
	AuditActionTagPolicyCreated      = "tag_policy.created"
	AuditActionTagPolicyDeleted      = "tag_policy.deleted"

	// Workflows (lifecycle).
	AuditActionWorkflowCreated         = "workflow.created"
	AuditActionWorkflowUpdated         = "workflow.updated"
	AuditActionWorkflowUpdatedBreaking = "workflow.updated_breaking"
	AuditActionWorkflowDeleted         = "workflow.deleted"
	AuditActionWorkflowTriggered       = "workflow.triggered"
	AuditActionWorkflowDryRun          = "workflow.dry_run"
	AuditActionWorkflowPlanRequested   = "workflow.plan_requested"
	AuditActionWorkflowCloned          = "workflow.cloned"
	AuditActionWorkflowsExported       = "workflows.exported"

	// Workflow runs.
	AuditActionWorkflowRunCancelled       = "workflow_run.cancelled"
	AuditActionWorkflowRunPaused          = "workflow_run.paused"
	AuditActionWorkflowRunResumed         = "workflow_run.resumed"
	AuditActionWorkflowRunRetried         = "workflow_run.retried"
	AuditActionWorkflowRunSubtreeReplayed = "workflow_run.subtree_replayed"
	AuditActionWorkflowRunBulkCancelled   = "workflow_run.bulk_cancelled"
	AuditActionWorkflowRunBulkReplayed    = "workflow_run.bulk_replayed"
	AuditActionWorkflowRunCompensated     = "workflow_run.compensated"

	// Workflow steps.
	AuditActionWorkflowStepApproved       = "workflow_step.approved"
	AuditActionWorkflowStepSkipped        = "workflow_step.skipped"
	AuditActionWorkflowStepForceCompleted = "workflow_step.force_completed"
	AuditActionWorkflowStepRetried        = "workflow_step.retried"

	// Workflow policies + canary.
	AuditActionWorkflowPolicyUpserted     = "workflow_policy.upserted"
	AuditActionCanaryDeploymentCreated    = "canary_deployment.created"
	AuditActionCanaryDeploymentUpdated    = "canary_deployment.updated"
	AuditActionCanaryDeploymentRolledBack = "canary_deployment.rolled_back"

	// Webhooks.
	AuditActionWebhookTested                   = "webhook.tested"
	AuditActionWebhookDeliveryReplayed         = "webhook.delivery_replayed"
	AuditActionWebhookDeliveryRetried          = "webhook_delivery.retried"
	AuditActionWebhookSubscriptionCreated      = "webhook_subscription.created"
	AuditActionWebhookSubscriptionDeleted      = "webhook_subscription.deleted"
	AuditActionWebhookSubscriptionRotateSecret = "webhook_subscription.rotate_secret"

	// Log drains.
	AuditActionLogDrainCreated = "log_drain.created"
	AuditActionLogDrainUpdated = "log_drain.updated"
	AuditActionLogDrainDeleted = "log_drain.deleted"

	// Notification channels.
	AuditActionNotificationChannelCreated = "notification_channel.created"
	AuditActionNotificationChannelUpdated = "notification_channel.updated"
	AuditActionNotificationChannelDeleted = "notification_channel.deleted"

	// Event sources + subscriptions + triggers.
	AuditActionEventSourceCreated       = "event_source.created"
	AuditActionEventSourceUpdated       = "event_source.updated"
	AuditActionEventSourceDeleted       = "event_source.deleted"
	AuditActionEventSourceSubscribed    = "event_source.subscribed"
	AuditActionEventSourceDispatched    = "event_source.dispatched"
	AuditActionEventSubscriptionDeleted = "event_subscription.deleted"
	AuditActionEventSent                = "event.sent"
	AuditActionEventSentByPrefix        = "event.sent_by_prefix"
	AuditActionEventTriggerCancelled    = "event_trigger.cancelled"
	AuditActionEventTriggerPurged       = "event_trigger.purged"

	// Deployments.
	AuditActionDeploymentVersionCreated    = "deployment_version.created"
	AuditActionDeploymentVersionFinalized  = "deployment_version.finalized"
	AuditActionDeploymentVersionPromoted   = "deployment_version.promoted"
	AuditActionDeploymentVersionRolledBack = "deployment_version.rolled_back"

	// Job endpoints (HMAC signing surface).
	AuditActionEndpointSet      = "endpoint.set"
	AuditActionEndpointVerified = "endpoint.verified"

	// Billing / usage / org settings.
	AuditActionSpendingLimitUpdated    = "spending_limit.updated"
	AuditActionEmailPreferencesUpdated = "email_preferences.updated"
	AuditActionUsageExported           = "usage.exported"
	AuditActionProjectBudgetUpdated    = "project_budget.updated"
	AuditActionAnomalyConfigUpdated    = "anomaly_config.updated"

	// Worker connections (gRPC streaming).
	AuditActionWorkerConnected         = "worker.connected"
	AuditActionWorkerDisconnected      = "worker.disconnected"
	AuditActionWorkerForceDisconnected = "worker.force_disconnected"
	AuditActionWorkerTaskRouted        = "worker.task_routed"
	AuditActionWorkerDeleteAcked       = "worker.delete.acked"
	AuditActionWorkerDeleteTimeout     = "worker.delete.timeout"

	// Quota and cron lifecycle (billing-period enforcement).
	AuditActionQuotaExceeded       = "quota.exceeded"
	AuditActionCronPausedQuota     = "cron.paused_quota"
	AuditActionCronResumedQuota    = "cron.resumed_quota"
	AuditActionSubscriptionChanged = "subscription.changed"
)

// allAuditActions is the set of every action name the emit path will accept.
// It is built from the const block above so adding a new const automatically
// widens the set. The coverage guard test verifies every handler uses one of
// these constants — so the registry is both the allowlist and the taxonomy.
var allAuditActions = map[string]struct{}{
	AuditActionAPIKeyCreated:                   {},
	AuditActionAPIKeyRevoked:                   {},
	AuditActionAPIKeyRotated:                   {},
	AuditActionAPIKeyListRead:                  {},
	AuditActionAuthRunTokenRejected:            {},
	AuditActionAuditExported:                   {},
	AuditActionAuditExportCapped:               {},
	AuditActionAuditListRead:                   {},
	AuditActionAuditSingleRead:                 {},
	AuditActionAuditChainVerified:              {},
	AuditActionKeyRotated:                      {},
	AuditActionRetentionTrimmed:                {},
	AuditActionDeadletterRead:                  {},
	AuditActionDeadletterReplayed:              {},
	AuditActionDeadletterDropped:               {},
	AuditActionDeadletterAged:                  {},
	AuditActionExportCapUpdated:                {},
	AuditActionRetentionUpdated:                {},
	AuditActionDeviceCodeApproved:              {},
	AuditActionProjectCreated:                  {},
	AuditActionProjectDeleted:                  {},
	AuditActionProjectSettingsUpdated:          {},
	AuditActionEnvironmentCreated:              {},
	AuditActionEnvironmentUpdated:              {},
	AuditActionEnvironmentDeleted:              {},
	AuditActionJobCreated:                      {},
	AuditActionJobUpdated:                      {},
	AuditActionJobCloned:                       {},
	AuditActionJobDeleted:                      {},
	AuditActionJobPaused:                       {},
	AuditActionJobResumed:                      {},
	AuditActionJobBatchCreated:                 {},
	AuditActionJobBatchEnabled:                 {},
	AuditActionJobBatchDisabled:                {},
	AuditActionJobTriggered:                    {},
	AuditActionJobBulkTriggered:                {},
	AuditActionJobsExported:                    {},
	AuditActionJobGroupCreated:                 {},
	AuditActionJobGroupUpdated:                 {},
	AuditActionJobGroupDeleted:                 {},
	AuditActionJobGroupPausedAll:               {},
	AuditActionJobGroupResumedAll:              {},
	AuditActionJobDependencyCreated:            {},
	AuditActionJobDependencyDeleted:            {},
	AuditActionRunCancelled:                    {},
	AuditActionRunReplayed:                     {},
	AuditActionRunReplayedDeadletter:           {},
	AuditActionRunBulkReplayedDeadletter:       {},
	AuditActionRunDebugModeSet:                 {},
	AuditActionRunIdempotencyKeyReset:          {},
	AuditActionRunRescheduled:                  {},
	AuditActionRunBulkReplayed:                 {},
	AuditActionRunBulkCancelled:                {},
	AuditActionRunBulkCancelledAll:             {},
	AuditActionRunPaused:                       {},
	AuditActionRunResumed:                      {},
	AuditActionRunRestarted:                    {},
	AuditActionRunsExported:                    {},
	AuditActionSecretCreated:                   {},
	AuditActionSecretDeleted:                   {},
	AuditActionSecretListRead:                  {},
	AuditActionSecretRead:                      {},
	AuditActionSSETokenCreated:                 {},
	AuditActionRoleCreated:                     {},
	AuditActionRoleUpdated:                     {},
	AuditActionRoleDeleted:                     {},
	AuditActionRoleSystemSeeded:                {},
	AuditActionPermissionGranted:               {},
	AuditActionPermissionRevoked:               {},
	AuditActionResourcePolicyCreated:           {},
	AuditActionResourcePolicyDeleted:           {},
	AuditActionTagPolicyCreated:                {},
	AuditActionTagPolicyDeleted:                {},
	AuditActionWorkflowCreated:                 {},
	AuditActionWorkflowUpdated:                 {},
	AuditActionWorkflowUpdatedBreaking:         {},
	AuditActionWorkflowDeleted:                 {},
	AuditActionWorkflowTriggered:               {},
	AuditActionWorkflowDryRun:                  {},
	AuditActionWorkflowPlanRequested:           {},
	AuditActionWorkflowCloned:                  {},
	AuditActionWorkflowsExported:               {},
	AuditActionWorkflowRunCancelled:            {},
	AuditActionWorkflowRunPaused:               {},
	AuditActionWorkflowRunResumed:              {},
	AuditActionWorkflowRunRetried:              {},
	AuditActionWorkflowRunSubtreeReplayed:      {},
	AuditActionWorkflowRunBulkCancelled:        {},
	AuditActionWorkflowRunBulkReplayed:         {},
	AuditActionWorkflowRunCompensated:          {},
	AuditActionWorkflowStepApproved:            {},
	AuditActionWorkflowStepSkipped:             {},
	AuditActionWorkflowStepForceCompleted:      {},
	AuditActionWorkflowStepRetried:             {},
	AuditActionWorkflowPolicyUpserted:          {},
	AuditActionCanaryDeploymentCreated:         {},
	AuditActionCanaryDeploymentUpdated:         {},
	AuditActionCanaryDeploymentRolledBack:      {},
	AuditActionWebhookTested:                   {},
	AuditActionWebhookDeliveryReplayed:         {},
	AuditActionWebhookDeliveryRetried:          {},
	AuditActionWebhookSubscriptionCreated:      {},
	AuditActionWebhookSubscriptionDeleted:      {},
	AuditActionWebhookSubscriptionRotateSecret: {},
	AuditActionLogDrainCreated:                 {},
	AuditActionLogDrainUpdated:                 {},
	AuditActionLogDrainDeleted:                 {},
	AuditActionNotificationChannelCreated:      {},
	AuditActionNotificationChannelUpdated:      {},
	AuditActionNotificationChannelDeleted:      {},
	AuditActionEventSourceCreated:              {},
	AuditActionEventSourceUpdated:              {},
	AuditActionEventSourceDeleted:              {},
	AuditActionEventSourceSubscribed:           {},
	AuditActionEventSourceDispatched:           {},
	AuditActionEventSubscriptionDeleted:        {},
	AuditActionEventSent:                       {},
	AuditActionEventSentByPrefix:               {},
	AuditActionEventTriggerCancelled:           {},
	AuditActionEventTriggerPurged:              {},
	AuditActionDeploymentVersionCreated:        {},
	AuditActionDeploymentVersionFinalized:      {},
	AuditActionDeploymentVersionPromoted:       {},
	AuditActionDeploymentVersionRolledBack:     {},
	AuditActionEndpointSet:                     {},
	AuditActionEndpointVerified:                {},
	AuditActionSpendingLimitUpdated:            {},
	AuditActionEmailPreferencesUpdated:         {},
	AuditActionUsageExported:                   {},
	AuditActionProjectBudgetUpdated:            {},
	AuditActionAnomalyConfigUpdated:            {},
	AuditActionWorkerConnected:                 {},
	AuditActionWorkerDisconnected:              {},
	AuditActionWorkerForceDisconnected:         {},
	AuditActionWorkerTaskRouted:                {},
	AuditActionWorkerDeleteAcked:               {},
	AuditActionWorkerDeleteTimeout:             {},
	AuditActionQuotaExceeded:                   {},
	AuditActionCronPausedQuota:                 {},
	AuditActionCronResumedQuota:                {},
	AuditActionSubscriptionChanged:             {},
}

// IsKnownAuditAction reports whether action is a registered audit action.
// The emit path calls this to reject typos at runtime before they reach
// the database.
func IsKnownAuditAction(action string) bool {
	_, ok := allAuditActions[action]
	return ok
}

// KnownAuditActions returns a copy of the registered action set. Useful for
// tests and for the JSON schema generator.
func KnownAuditActions() []string {
	out := make([]string, 0, len(allAuditActions))
	for a := range allAuditActions {
		out = append(out, a)
	}
	return out
}
