package domain

// AuditActionSchema describes the expected shape of an audit event's details
// payload for a given action. Required keys must be present and non-zero
// when the event is emitted. Forbidden keys must never be present, even if
// empty — they exist as a defense-in-depth control against leaking secret
// values by name (e.g. details["password"] = "..." would fail the test even
// if the value is empty).
//
// This schema is the authoritative contract for downstream consumers. The
// tests in internal/api/audit_detail_schema_test.go enforce it.
type AuditActionSchema struct {
	// Required keys that must appear in details with a non-zero value.
	Required []string
	// Forbidden keys that must never appear in details. Values are
	// irrelevant — presence alone is a failure.
	Forbidden []string
	// Description is a short human-readable summary of the action for
	// compliance reports and SIEM integrations.
	Description string
}

// commonForbiddenKeys is the baseline set of keys no audit event should
// ever carry. Any action schema's Forbidden list is unioned with this set.
var commonForbiddenKeys = []string{
	"secret",
	"value",
	"password",
	"private_key",
	"auth_header",
	"webhook_secret",
	"api_key_plaintext",
	"raw_key",
	"token",
	"bearer",
}

// AuditActionSchemas maps every registered audit action to its schema.
// Every action in allAuditActions must have an entry here (enforced by
// TestAuditActionSchemas_ComplereCoverage). Adding a new action requires
// adding both the const (audit_actions.go) and the schema (here).
var AuditActionSchemas = map[string]AuditActionSchema{
	// API keys.
	AuditActionAPIKeyCreated: {
		Required:    []string{"name", "key_prefix"},
		Forbidden:   []string{"key", "rawKey"},
		Description: "API key created; records name, prefix, scopes, expiry — never the plaintext.",
	},
	AuditActionAPIKeyRevoked: {
		Description: "API key revoked.",
	},
	AuditActionAPIKeyRotated: {
		Required:    []string{"new_key_id"},
		Forbidden:   []string{"key", "rawKey"},
		Description: "API key rotated to a new key id; old key retained for a grace period.",
	},
	AuditActionAPIKeyListRead: {
		Description: "API key list read.",
	},
	AuditActionAuthRunTokenRejected: {
		Required:    []string{"reason", "run_id"},
		Description: "SDK run-token authentication rejected before run-scoped API access.",
	},

	// Audit.
	AuditActionAuditExported: {
		Required:    []string{"from", "to", "format"},
		Description: "Audit log exported by a caller with audit:export scope.",
	},
	AuditActionAuditExportCapped: {
		Required:    []string{"exported"},
		Description: "Audit export terminated early due to row cap.",
	},
	AuditActionAuditListRead: {
		Description: "Audit log list read.",
	},
	AuditActionAuditSingleRead: {
		Required:    []string{"target_id"},
		Description: "Single audit event read.",
	},
	AuditActionAuditChainVerified: {
		Description: "Audit chain verification endpoint was invoked.",
	},
	AuditActionKeyRotated: {
		Required:    []string{"previous_epoch", "new_epoch", "rotated_by"},
		Forbidden:   []string{"key", "new_key", "secret_material"},
		Description: "Audit HMAC signing key rotation; writes a forensic anchor row that boundary-stitches the chain across epochs.",
	},
	AuditActionRetentionTrimmed: {
		Required:    []string{"deleted_count", "trimmed_before", "previous_hash"},
		Description: "Retention reaper trimmed old audit rows; writes a tombstone anchor row giving positive forensic proof of the trim (count, cutoff, and chain tail at trim time).",
	},
	AuditActionDeadletterRead: {
		Required:    []string{"filter"},
		Description: "Admin operator listed audit deadletter queue entries. Filter records the serialized query params with any secret-shaped values redacted.",
	},
	AuditActionDeadletterReplayed: {
		Required:    []string{"deadletter_id", "new_event_id"},
		Description: "Admin operator manually replayed a deadletter entry into the audit chain; DLQ row deleted on success.",
	},
	AuditActionDeadletterDropped: {
		Required:    []string{"deadletter_id", "reason"},
		Description: "Admin operator permanently dropped a deadletter entry, accepting data loss.",
	},
	AuditActionDeadletterAged: {
		Required:    []string{"dropped_count", "reason"},
		Description: "Audit deadletter retention reaper dropped rows older than the configured AUDIT_DLQ_MAX_AGE_DAYS window; reason carries the trigger (e.g. max_age_exceeded).",
	},
	AuditActionExportCapUpdated: {
		Required:    []string{"old_cap", "new_cap"},
		Description: "Admin operator updated the per-project audit export row cap. old_cap and new_cap are BIGINT values; 0 denotes inherit-from-default.",
	},
	AuditActionRetentionUpdated: {
		Required:    []string{"old_days", "new_days"},
		Description: "Admin operator updated the per-project audit retention window (days). 0 disables retention trimming for the project; a negative value is rejected at the API layer.",
	},

	// Device code.
	AuditActionDeviceCodeApproved: {
		Required:    []string{"user_code", "api_key_id"},
		Description: "CLI device code approved; a new API key is issued.",
	},

	// Projects.
	AuditActionProjectCreated: {
		Required:    []string{"name", "org_id"},
		Description: "Project created.",
	},
	AuditActionProjectDeleted: {
		Description: "Project deleted.",
	},
	AuditActionProjectSettingsUpdated: {
		Required:    []string{"changes"},
		Description: "Project-level settings updated (default region, key lifetime, etc.).",
	},

	// Environments.
	AuditActionEnvironmentCreated: {
		Required:    []string{"name", "slug"},
		Forbidden:   []string{"variables"},
		Description: "Environment created. Variable keys only are recorded, never values.",
	},
	AuditActionEnvironmentUpdated: {
		Required:    []string{"changed_fields"},
		Forbidden:   []string{"variables"},
		Description: "Environment updated.",
	},
	AuditActionEnvironmentDeleted: {
		Description: "Environment deleted.",
	},

	// Jobs.
	AuditActionJobCreated: {
		Required:    []string{"name", "slug", "execution_mode"},
		Description: "Job definition created.",
	},
	AuditActionJobUpdated: {
		Required:    []string{"changes"},
		Description: "Job definition updated.",
	},
	AuditActionJobCloned: {
		Required:    []string{"source_job_id", "new_name", "new_slug"},
		Description: "Job cloned from an existing definition.",
	},
	AuditActionJobDeleted: {
		Description: "Job deleted.",
	},
	AuditActionJobPaused: {
		Description: "Job paused.",
	},
	AuditActionJobResumed: {
		Description: "Job resumed.",
	},
	AuditActionJobBatchCreated: {
		Required:    []string{"count"},
		Description: "Batch job creation.",
	},
	AuditActionJobBatchEnabled: {
		Required:    []string{"count"},
		Description: "Batch job enablement.",
	},
	AuditActionJobBatchDisabled: {
		Required:    []string{"count"},
		Description: "Batch job disablement.",
	},
	AuditActionJobTriggered: {
		Required:    []string{"run_id"},
		Description: "Job run triggered via API (hot path, async audit).",
	},
	AuditActionJobBulkTriggered: {
		Required:    []string{"batch_id", "total"},
		Description: "Bulk job triggering (hot path, async audit).",
	},
	AuditActionJobsExported: {
		Required:    []string{"format", "project_id"},
		Description: "Job definitions exported as a stream.",
	},

	// Job groups.
	AuditActionJobGroupCreated: {
		Required:    []string{"name", "slug"},
		Description: "Job group created.",
	},
	AuditActionJobGroupUpdated: {
		Description: "Job group updated.",
	},
	AuditActionJobGroupDeleted: {
		Description: "Job group deleted.",
	},
	AuditActionJobGroupPausedAll: {
		Description: "Every job in a group paused.",
	},
	AuditActionJobGroupResumedAll: {
		Description: "Every job in a group resumed.",
	},

	// Job dependencies.
	AuditActionJobDependencyCreated: {
		Required:    []string{"job_id", "depends_on_job_id", "condition"},
		Description: "Job dependency edge created.",
	},
	AuditActionJobDependencyDeleted: {
		Required:    []string{"job_id"},
		Description: "Job dependency edge deleted.",
	},

	// Runs.
	AuditActionRunCancelled: {
		Required:    []string{"job_id"},
		Description: "Run cancelled by user.",
	},
	AuditActionRunReplayed: {
		Required:    []string{"original_run_id", "job_id"},
		Description: "Run replayed.",
	},
	AuditActionRunReplayedDeadletter: {
		Required:    []string{"job_id"},
		Description: "Dead-lettered run replayed.",
	},
	AuditActionRunBulkReplayedDeadletter: {
		Required:    []string{"count"},
		Description: "Bulk dead-letter replay.",
	},
	AuditActionRunDebugModeSet: {
		Required:    []string{"enabled"},
		Description: "Run debug mode toggled.",
	},
	AuditActionRunIdempotencyKeyReset: {
		Description: "Run idempotency key reset so the same key can be reused.",
	},
	AuditActionRunRescheduled: {
		Required:    []string{"job_id", "new_scheduled_at"},
		Description: "Run rescheduled.",
	},
	AuditActionRunBulkReplayed: {
		Required:    []string{"count"},
		Description: "Bulk run replay.",
	},
	AuditActionRunBulkCancelled: {
		Required:    []string{"total"},
		Description: "Bulk run cancellation by explicit id list.",
	},
	AuditActionRunBulkCancelledAll: {
		Required:    []string{"project_id"},
		Description: "Bulk run cancellation by filter.",
	},
	AuditActionRunPaused: {
		Required:    []string{"job_id"},
		Description: "Run paused.",
	},
	AuditActionRunResumed: {
		Required:    []string{"job_id"},
		Description: "Run resumed.",
	},
	AuditActionRunRestarted: {
		Required:    []string{"job_id"},
		Description: "Run restarted.",
	},
	AuditActionRunsExported: {
		Required:    []string{"format", "from", "to", "project_id"},
		Description: "Job run history exported as a stream.",
	},

	// Secrets.
	AuditActionSecretCreated: {
		Required:    []string{"secret_key"},
		Forbidden:   []string{"value", "encrypted_value", "plaintext"},
		Description: "Secret created. Key name only; never the value.",
	},
	AuditActionSecretDeleted: {
		Required:    []string{"secret_key"},
		Forbidden:   []string{"value", "encrypted_value", "plaintext"},
		Description: "Secret deleted.",
	},
	AuditActionSecretListRead: {
		Description: "Secret list read.",
	},
	AuditActionSecretRead: {
		Required:    []string{"secret_id", "name"},
		Forbidden:   []string{"value", "encrypted_value", "plaintext", "key", "key_material"},
		Description: "Single secret metadata read (never the decrypted value).",
	},

	// SSE.
	AuditActionSSETokenCreated: {
		Required:    []string{"expires_at"},
		Forbidden:   []string{"token"},
		Description: "Short-lived SSE token issued.",
	},

	// RBAC.
	AuditActionRoleCreated: {
		Required:    []string{"name", "permissions"},
		Description: "RBAC role created.",
	},
	AuditActionRoleUpdated: {
		Required:    []string{"changes"},
		Description: "RBAC role updated.",
	},
	AuditActionRoleDeleted: {
		Description: "RBAC role deleted.",
	},
	AuditActionRoleSystemSeeded: {
		Required:    []string{"project_id"},
		Description: "System roles seeded for a project.",
	},
	AuditActionPermissionGranted: {
		Required:    []string{"user_id", "project_id"},
		Description: "Role assigned to a member (permission grant).",
	},
	AuditActionPermissionRevoked: {
		Required:    []string{"user_id", "project_id"},
		Description: "Role removed from a member (permission revocation).",
	},
	AuditActionResourcePolicyCreated: {
		Required:    []string{"resource_type", "resource_id", "user_id", "actions"},
		Description: "Resource-level policy created.",
	},
	AuditActionResourcePolicyDeleted: {
		Description: "Resource-level policy deleted.",
	},
	AuditActionTagPolicyCreated: {
		Required:    []string{"tag_key", "resource_type", "user_id", "actions"},
		Description: "Tag-based policy created.",
	},
	AuditActionTagPolicyDeleted: {
		Description: "Tag-based policy deleted.",
	},

	// Workflows.
	AuditActionWorkflowCreated: {
		Required:    []string{"name", "slug"},
		Description: "Workflow DAG created.",
	},
	AuditActionWorkflowUpdated: {
		Required:    []string{"new_version"},
		Description: "Workflow updated (non-breaking).",
	},
	AuditActionWorkflowUpdatedBreaking: {
		Required:    []string{"previous_version_id", "new_version"},
		Description: "Workflow updated with a breaking change while active runs exist on the previous version.",
	},
	AuditActionWorkflowDeleted: {
		Description: "Workflow deleted.",
	},
	AuditActionWorkflowTriggered: {
		Required:    []string{"run_id"},
		Description: "Workflow run triggered (hot path, async audit).",
	},
	AuditActionWorkflowDryRun: {
		Description: "Workflow DAG dry-run validation.",
	},
	AuditActionWorkflowPlanRequested: {
		Description: "Workflow execution plan requested.",
	},
	AuditActionWorkflowCloned: {
		Required:    []string{"source_workflow_id", "new_name"},
		Description: "Workflow cloned from an existing definition.",
	},
	AuditActionWorkflowsExported: {
		Required:    []string{"format", "project_id"},
		Description: "Workflow definitions exported as a stream.",
	},

	// Workflow runs.
	AuditActionWorkflowRunCancelled: {
		Required:    []string{"workflow_id"},
		Description: "Workflow run cancelled.",
	},
	AuditActionWorkflowRunPaused: {
		Required:    []string{"workflow_id"},
		Description: "Workflow run paused.",
	},
	AuditActionWorkflowRunResumed: {
		Required:    []string{"workflow_id"},
		Description: "Workflow run resumed.",
	},
	AuditActionWorkflowRunRetried: {
		Required:    []string{"original_run_id"},
		Description: "Workflow run retried into a new run id.",
	},
	AuditActionWorkflowRunSubtreeReplayed: {
		Required:    []string{"from_step_ref"},
		Description: "Workflow subtree replayed from a specific step.",
	},
	AuditActionWorkflowRunBulkCancelled: {
		Required:    []string{"total"},
		Description: "Bulk workflow run cancellation.",
	},
	AuditActionWorkflowRunBulkReplayed: {
		Required:    []string{"total"},
		Description: "Bulk workflow run replay.",
	},
	AuditActionWorkflowRunCompensated: {
		Required:    []string{"workflow_id"},
		Description: "Workflow compensation (saga rollback) triggered.",
	},

	// Workflow steps.
	AuditActionWorkflowStepApproved: {
		Required:    []string{"workflow_run_id", "step_ref", "approver"},
		Description: "Workflow approval gate approved.",
	},
	AuditActionWorkflowStepSkipped: {
		Required:    []string{"workflow_run_id", "step_ref"},
		Description: "Workflow step skipped.",
	},
	AuditActionWorkflowStepForceCompleted: {
		Required:    []string{"workflow_run_id", "step_ref"},
		Description: "Workflow step force-completed by an operator.",
	},
	AuditActionWorkflowStepRetried: {
		Required:    []string{"workflow_run_id", "step_ref"},
		Description: "Workflow step retried.",
	},

	// Workflow policies + canary.
	AuditActionWorkflowPolicyUpserted: {
		Required:    []string{"project_id"},
		Description: "Workflow DAG policy upserted.",
	},
	AuditActionCanaryDeploymentCreated: {
		Required:    []string{"workflow_id", "source_version", "target_version", "traffic_pct"},
		Description: "Workflow canary deployment created.",
	},
	AuditActionCanaryDeploymentUpdated: {
		Required:    []string{"workflow_id", "traffic_pct"},
		Description: "Workflow canary traffic adjusted.",
	},
	AuditActionCanaryDeploymentRolledBack: {
		Required:    []string{"workflow_id"},
		Description: "Workflow canary rolled back.",
	},

	// Webhooks.
	AuditActionWebhookTested: {
		Required:    []string{"url_host"},
		Forbidden:   []string{"url", "secret"},
		Description: "Webhook URL tested. Host only, never full URL or secret.",
	},
	AuditActionWebhookDeliveryReplayed: {
		Description: "Webhook delivery replayed.",
	},
	AuditActionWebhookDeliveryRetried: {
		Required:    []string{"subscription_id"},
		Description: "Webhook delivery manually retried.",
	},
	AuditActionWebhookSubscriptionCreated: {
		Required:    []string{"url_host", "event_types"},
		Forbidden:   []string{"url", "secret", "webhook_secret"},
		Description: "Webhook subscription created.",
	},
	AuditActionWebhookSubscriptionDeleted: {
		Required:    []string{"url_host"},
		Forbidden:   []string{"url", "secret"},
		Description: "Webhook subscription deleted.",
	},
	AuditActionWebhookSubscriptionRotateSecret: {
		Required:    []string{"grace_expires_at", "grace_period_minutes"},
		Forbidden:   []string{"secret", "new_secret"},
		Description: "Webhook signing secret rotated.",
	},

	// Log drains.
	AuditActionLogDrainCreated: {
		Required:    []string{"name", "drain_type"},
		Forbidden:   []string{"endpoint_url", "auth_config", "auth_token"},
		Description: "Log drain created. Endpoint host only, auth config redacted.",
	},
	AuditActionLogDrainUpdated: {
		Required:    []string{"changed_fields"},
		Forbidden:   []string{"endpoint_url", "auth_config", "auth_token"},
		Description: "Log drain updated.",
	},
	AuditActionLogDrainDeleted: {
		Description: "Log drain deleted.",
	},

	// Notification channels.
	AuditActionNotificationChannelCreated: {
		Required:    []string{"name", "channel_type"},
		Forbidden:   []string{"config", "webhook_url", "bot_token"},
		Description: "Notification channel created.",
	},
	AuditActionNotificationChannelUpdated: {
		Required:    []string{"name", "channel_type"},
		Forbidden:   []string{"config", "webhook_url", "bot_token"},
		Description: "Notification channel updated.",
	},
	AuditActionNotificationChannelDeleted: {
		Description: "Notification channel deleted.",
	},

	// Event sources + subscriptions + triggers.
	AuditActionEventSourceCreated: {
		Required:    []string{"name"},
		Forbidden:   []string{"signature_secret", "signature_secret_enc"},
		Description: "Event source created.",
	},
	AuditActionEventSourceUpdated: {
		Required:    []string{"changed_fields"},
		Forbidden:   []string{"signature_secret", "signature_secret_enc"},
		Description: "Event source updated.",
	},
	AuditActionEventSourceDeleted: {
		Description: "Event source deleted.",
	},
	AuditActionEventSourceSubscribed: {
		Required:    []string{"subscription_id", "target_type", "target_id"},
		Description: "Target subscribed to an event source.",
	},
	AuditActionEventSourceDispatched: {
		Required:    []string{"source_name", "dispatched"},
		Description: "Event dispatched to subscribers (hot path, async audit).",
	},
	AuditActionEventSubscriptionDeleted: {
		Required:    []string{"source_id"},
		Description: "Event subscription deleted.",
	},
	AuditActionEventSent: {
		Required:    []string{"event_key"},
		Forbidden:   []string{"payload"},
		Description: "Event sent to a waiting trigger.",
	},
	AuditActionEventSentByPrefix: {
		Required:    []string{"prefix"},
		Forbidden:   []string{"payload"},
		Description: "Events sent to all triggers matching a key prefix.",
	},
	AuditActionEventTriggerCancelled: {
		Required:    []string{"event_key"},
		Description: "Event trigger cancelled.",
	},
	AuditActionEventTriggerPurged: {
		Required:    []string{"older_than_days"},
		Description: "Event triggers purged.",
	},

	// Deployment versions.
	AuditActionDeploymentVersionCreated: {
		Required:    []string{"environment"},
		Description: "Artifact deployment version created.",
	},
	AuditActionDeploymentVersionFinalized: {
		Required:    []string{"environment"},
		Description: "Artifact deployment version finalized.",
	},
	AuditActionDeploymentVersionPromoted: {
		Required:    []string{"environment"},
		Description: "Artifact deployment version promoted.",
	},
	AuditActionDeploymentVersionRolledBack: {
		Required:    []string{"environment"},
		Description: "Artifact deployment version rolled back.",
	},

	// Job endpoints.
	AuditActionEndpointSet: {
		Required:    []string{"job_id", "endpoint_url_host"},
		Forbidden:   []string{"endpoint_signing_secret", "signing_secret"},
		Description: "Job HTTP endpoint URL updated; a fresh HMAC signing secret was generated.",
	},
	AuditActionEndpointVerified: {
		Required:    []string{"job_id", "endpoint_url_host", "success"},
		Forbidden:   []string{"endpoint_signing_secret", "signing_secret"},
		Description: "Signed test ping sent to the job endpoint; result recorded.",
	},

	// Billing / usage.
	AuditActionSpendingLimitUpdated: {
		Required:    []string{"limit_microusd", "action"},
		Description: "Org spending limit updated.",
	},
	AuditActionEmailPreferencesUpdated: {
		Required:    []string{"monthly_usage_email"},
		Description: "Org email preferences updated.",
	},
	AuditActionUsageExported: {
		Required:    []string{"format", "from", "to"},
		Description: "Org usage report exported.",
	},
	AuditActionProjectBudgetUpdated: {
		Required:    []string{"budget_microusd", "action"},
		Description: "Project budget updated.",
	},
	AuditActionAnomalyConfigUpdated: {
		Required:    []string{"warning_threshold", "critical_threshold"},
		Description: "Anomaly detection thresholds updated.",
	},

	// Worker connections (gRPC streaming).
	AuditActionWorkerConnected: {
		Required:    []string{"worker_id", "hostname"},
		Description: "Worker connected via gRPC streaming.",
	},
	AuditActionWorkerDisconnected: {
		Required:    []string{"worker_id"},
		Description: "Worker disconnected from gRPC stream.",
	},
	AuditActionWorkerForceDisconnected: {
		Required:    []string{"worker_id", "reason"},
		Description: "Worker force-disconnected by operator or revocation broadcast.",
	},
	AuditActionWorkerTaskRouted: {
		Required:    []string{"run_id", "worker_id", "queue", "project_id"},
		Description: "Worker-mode run routed to a connected gRPC worker.",
	},
	AuditActionWorkerDeleteAcked: {
		Required:    []string{"worker_id"},
		Description: "Worker force-disconnect request was acknowledged by the worker-plane replica.",
	},
	AuditActionWorkerDeleteTimeout: {
		Required:    []string{"worker_id", "timeout_ms"},
		Description: "Worker force-disconnect request timed out waiting for worker-plane acknowledgement.",
	},

	// Quota and cron lifecycle (billing-period enforcement).
	AuditActionQuotaExceeded: {
		Required:    []string{"org_id", "plan_tier"},
		Description: "Org exceeded its quota; cron jobs paused until the next billing period.",
	},
	AuditActionCronPausedQuota: {
		Required:    []string{"org_id", "jobs_paused"},
		Description: "Cron jobs paused automatically because the org's quota was exceeded.",
	},
	AuditActionCronResumedQuota: {
		Required:    []string{"org_id", "jobs_resumed"},
		Description: "Cron jobs resumed automatically at the start of a new billing period after quota reset.",
	},
	AuditActionSubscriptionChanged: {
		Required:    []string{"org_id", "plan_tier"},
		Description: "Org subscription changed (plan upgrade, downgrade, or renewal).",
	},
	AuditActionUsageThresholdReached: {
		Required:    []string{"org_id", "plan_tier", "metric", "threshold_pct", "current", "limit"},
		Description: "Org crossed an 80%, 90%, or 100% threshold of a metered quota in the current billing period. Emitted at most once per (org, metric, threshold) per period.",
	},

	// Internal-secret bypass.
	AuditActionInternalSecretBypass: {
		Required:    []string{"gate", "caller", "handler"},
		Description: "Project-scoped handler entered via X-Internal-Secret without a project context. gate names the skipped check, caller is the sender identity (api-key:<id> or 'unknown'), handler names the entry point.",
	},
}

// ForbiddenKeysFor returns the union of the action-specific forbidden keys
// and commonForbiddenKeys. Used by TestAuditDetailSchema.
func ForbiddenKeysFor(action string) []string {
	schema, ok := AuditActionSchemas[action]
	if !ok {
		return append([]string(nil), commonForbiddenKeys...)
	}
	out := make([]string, 0, len(schema.Forbidden)+len(commonForbiddenKeys))
	out = append(out, schema.Forbidden...)
	out = append(out, commonForbiddenKeys...)
	return out
}
