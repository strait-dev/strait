import type { ProjectPermissionFlags } from "@/hooks/auth/use-project-permissions";

export function jobResourcePermissions(permissions: ProjectPermissionFlags) {
  return {
    canCreate: permissions.canWriteJobs,
    canEdit: permissions.canWriteJobs,
    canDelete: permissions.canWriteJobs,
    canPauseResume: permissions.canWriteJobs,
    canTrigger: permissions.canTriggerJobs,
  };
}

export function scheduleResourcePermissions(
  permissions: ProjectPermissionFlags
) {
  return jobResourcePermissions(permissions);
}

export function workflowResourcePermissions(
  permissions: ProjectPermissionFlags
) {
  return {
    canCreate: permissions.canWriteWorkflows,
    canDelete: permissions.canWriteWorkflows,
    canPauseResume: permissions.canWriteWorkflows,
    canTrigger: permissions.canTriggerWorkflows,
  };
}

export function runResourcePermissions(permissions: ProjectPermissionFlags) {
  return {
    canCancel: permissions.canWriteRuns,
    canRetry: permissions.canWriteRuns,
  };
}

export function dlqResourcePermissions(permissions: ProjectPermissionFlags) {
  return {
    canDiscard: permissions.canWriteRuns,
    canRetry: permissions.canWriteRuns,
  };
}

export function webhookResourcePermissions(
  permissions: ProjectPermissionFlags
) {
  return {
    canCreate: permissions.canWriteWebhooks,
    canDelete: permissions.canWriteWebhooks,
  };
}
