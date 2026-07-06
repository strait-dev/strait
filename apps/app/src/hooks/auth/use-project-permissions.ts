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

import {
  emptyProjectPermissionFlags,
  flagsFromPermissions,
  type ProjectRole,
  type RoleWithLineageResponse,
  rolePermissions,
  SCOPE_ALL,
} from "./project-permissions";

export type { ProjectPermissionFlags } from "./project-permissions";

type ProjectMemberRole = {
  user_id: string;
  role_id: string;
};

type PaginatedResponse<T> = {
  data: T[];
  has_more?: boolean;
  next_cursor?: string;
};

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
