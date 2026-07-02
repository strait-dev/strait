import { queryOptions, useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { apiPath, apiRequest } from "@/lib/api-client.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  getOrganizationRole,
  getProjectOrganizationRole,
  requireActiveProjectAccess,
} from "@/middlewares/require-access";
import { queryKeys } from "../query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "../utils";

const SCOPE_ALL = "*";
const SCOPE_JOBS_WRITE = "jobs:write";
const SCOPE_JOBS_TRIGGER = "jobs:trigger";
const SCOPE_RUNS_WRITE = "runs:write";
const SCOPE_WORKFLOWS_WRITE = "workflows:write";
const SCOPE_WORKFLOWS_TRIGGER = "workflows:trigger";
const SCOPE_WEBHOOKS_WRITE = "webhooks:write";
const SCOPE_API_KEYS_MANAGE = "api-keys:manage";
const SCOPE_PROJECTS_WRITE = "projects:write";
const SCOPE_PROJECTS_MANAGE = "projects:manage";

type ProjectMemberRole = {
  user_id: string;
  role_id: string;
};

type ProjectRole = {
  permissions: string[] | null;
};

type PaginatedResponse<T> = {
  data: T[];
  has_more?: boolean;
  next_cursor?: string;
};

type RoleWithLineageResponse = {
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

const emptyProjectPermissionFlags: ProjectPermissionFlags = {
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

function hasScope(permissions: string[], scope: string) {
  return permissions.includes(SCOPE_ALL) || permissions.includes(scope);
}

function flagsFromPermissions(permissions: string[]): ProjectPermissionFlags {
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

function rolePermissions(response: ProjectRole | RoleWithLineageResponse) {
  if ("permissions" in response) {
    return response.permissions ?? [];
  }

  return [
    ...(response.role?.permissions ?? []),
    ...(response.lineage ?? []).flatMap((role) => role.permissions ?? []),
  ];
}

async function findProjectMembership(projectId: string, userId: string) {
  let cursor: string | undefined;

  for (;;) {
    const members = await apiRequest<PaginatedResponse<ProjectMemberRole>>(
      "/v1/members",
      {
        params: {
          limit: 100,
          ...(cursor ? { cursor } : {}),
        },
        projectId,
      }
    );
    const membership = members.data.find((member) => member.user_id === userId);

    if (membership || members.has_more !== true || !members.next_cursor) {
      return membership;
    }

    cursor = members.next_cursor;
  }
}

const fetchProjectPermissionsFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async ({ context }) => {
    const projectId = await requireActiveProjectAccess(context);
    const orgRole =
      (await getProjectOrganizationRole(context.user.id, projectId)) ??
      (context.activeOrganizationId
        ? await getOrganizationRole(
            context.user.id,
            context.activeOrganizationId
          )
        : null);

    if (orgRole === "owner" || orgRole === "admin") {
      return flagsFromPermissions([SCOPE_ALL]);
    }

    const membership = await findProjectMembership(projectId, context.user.id);

    if (!membership?.role_id) {
      return emptyProjectPermissionFlags;
    }

    const roleResponse = await apiRequest<
      ProjectRole | RoleWithLineageResponse
    >(apiPath`/v1/roles/${membership.role_id}`, {
      params: { include_lineage: true },
      projectId,
    });

    const permissions = [...new Set(rolePermissions(roleResponse))];
    return flagsFromPermissions(permissions);
  });

const projectPermissionsQueryOptions = (projectId?: string | null) =>
  queryOptions({
    queryKey: queryKeys.projectPermissions.detail(projectId ?? "none").queryKey,
    queryFn: () => fetchProjectPermissionsFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!projectId,
  });

export function useProjectPermissions(projectId?: string | null) {
  const isHydrated = useIsHydrated();
  const query = useQuery(projectPermissionsQueryOptions(projectId));

  return {
    ...query,
    isHydrated,
    permissions:
      isHydrated && query.data ? query.data : emptyProjectPermissionFlags,
  };
}
