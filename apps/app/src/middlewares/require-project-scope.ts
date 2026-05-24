import { apiPath, apiRequest } from "@/lib/api-client.server";
import {
  type AuthzContext,
  requireActiveProjectAccess,
} from "@/middlewares/require-access";

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

function hasScope(permissions: string[] | null | undefined, scope: string) {
  return permissions?.includes("*") || permissions?.includes(scope);
}

function roleResponseHasScope(
  response: ProjectRole | RoleWithLineageResponse,
  scope: string
): boolean {
  if ("permissions" in response) {
    return hasScope(response.permissions, scope) === true;
  }

  if (response.role || response.lineage) {
    return (
      hasScope(response.role?.permissions, scope) ||
      response.lineage?.some((role) => hasScope(role.permissions, scope)) ===
        true
    );
  }

  return false;
}

async function findProjectMembership(
  projectId: string,
  userId: string
): Promise<ProjectMemberRole | undefined> {
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

export async function requireActiveProjectScope(
  context: AuthzContext,
  scope: string
): Promise<string> {
  const projectId = await requireActiveProjectAccess(context);
  const membership = await findProjectMembership(projectId, context.user.id);

  if (!membership?.role_id) {
    throw new Error("Forbidden");
  }

  const roleResponse = await apiRequest<ProjectRole | RoleWithLineageResponse>(
    apiPath`/v1/roles/${membership.role_id}`,
    {
      params: { include_lineage: true },
      projectId,
    }
  );

  if (!roleResponseHasScope(roleResponse, scope)) {
    throw new Error("Forbidden");
  }

  return projectId;
}
