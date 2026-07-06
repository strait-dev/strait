/**
 * Pure permission-derivation logic for a project role.
 *
 * Kept free of server/React imports so the scope-to-UI-flag mapping can be unit
 * tested directly; the React Query hook lives in use-project-permissions.ts.
 */

export const SCOPE_ALL = "*";
const SCOPE_JOBS_WRITE = "jobs:write";
const SCOPE_JOBS_TRIGGER = "jobs:trigger";
const SCOPE_RUNS_WRITE = "runs:write";
const SCOPE_WORKFLOWS_WRITE = "workflows:write";
const SCOPE_WORKFLOWS_TRIGGER = "workflows:trigger";
const SCOPE_WEBHOOKS_WRITE = "webhooks:write";
const SCOPE_API_KEYS_MANAGE = "api-keys:manage";
const SCOPE_PROJECTS_WRITE = "projects:write";
const SCOPE_PROJECTS_MANAGE = "projects:manage";

export type ProjectRole = {
  permissions: string[] | null;
};

export type RoleWithLineageResponse = {
  role?: ProjectRole;
  lineage?: ProjectRole[];
};

export type ProjectPermissionFlags = {
  permissions: string[];
  canWriteJobs: boolean;
  canTriggerJobs: boolean;
  canWriteRuns: boolean;
  canWriteWorkflows: boolean;
  canTriggerWorkflows: boolean;
  canWriteWebhooks: boolean;
  canManageApiKeys: boolean;
  canWriteProjects: boolean;
  canManageProjects: boolean;
};

export const emptyProjectPermissionFlags: ProjectPermissionFlags = {
  permissions: [],
  canWriteJobs: false,
  canTriggerJobs: false,
  canWriteRuns: false,
  canWriteWorkflows: false,
  canTriggerWorkflows: false,
  canWriteWebhooks: false,
  canManageApiKeys: false,
  canWriteProjects: false,
  canManageProjects: false,
};

/** A scope is granted when present explicitly or via the "*" wildcard. */
export function hasScope(permissions: string[], scope: string) {
  return permissions.includes(SCOPE_ALL) || permissions.includes(scope);
}

/**
 * Derive the UI capability flags from a flat scope list. Write scopes imply the
 * matching trigger scope (a user who can edit jobs can also run them).
 */
export function flagsFromPermissions(
  permissions: string[]
): ProjectPermissionFlags {
  return {
    permissions,
    canWriteJobs: hasScope(permissions, SCOPE_JOBS_WRITE),
    canTriggerJobs:
      hasScope(permissions, SCOPE_JOBS_TRIGGER) ||
      hasScope(permissions, SCOPE_JOBS_WRITE),
    canWriteRuns: hasScope(permissions, SCOPE_RUNS_WRITE),
    canWriteWorkflows: hasScope(permissions, SCOPE_WORKFLOWS_WRITE),
    canTriggerWorkflows:
      hasScope(permissions, SCOPE_WORKFLOWS_TRIGGER) ||
      hasScope(permissions, SCOPE_WORKFLOWS_WRITE),
    canWriteWebhooks: hasScope(permissions, SCOPE_WEBHOOKS_WRITE),
    canManageApiKeys: hasScope(permissions, SCOPE_API_KEYS_MANAGE),
    canWriteProjects: hasScope(permissions, SCOPE_PROJECTS_WRITE),
    canManageProjects: hasScope(permissions, SCOPE_PROJECTS_MANAGE),
  };
}

/**
 * Flatten a role response into a de-duplicated scope list. Accepts either a flat
 * role or a role-with-lineage response, in which case the role's own scopes and
 * every inherited role's scopes are merged.
 */
export function rolePermissions(
  response: ProjectRole | RoleWithLineageResponse
): string[] {
  if ("permissions" in response) {
    return response.permissions ?? [];
  }

  return [
    ...(response.role?.permissions ?? []),
    ...(response.lineage ?? []).flatMap((role) => role.permissions ?? []),
  ];
}
