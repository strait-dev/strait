import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { auth } from "@/lib/auth.server";

export type MemberData = {
  id: string;
  userId: string;
  role: "owner" | "admin" | "member";
  email: string;
  name: string;
  image: string | null;
  createdAt: Date;
};

type MemberRole = "owner" | "admin" | "member";

const toDate = (value: unknown) =>
  value instanceof Date ? value : new Date(String(value));

const listMembersServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { organizationId: string }) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const organization = await auth.api.getFullOrganization({
      query: { organizationId: data.organizationId },
      headers,
    });

    if (!organization) {
      return [];
    }

    return (organization.members ?? []).map(
      (member: {
        id: string;
        userId: string;
        role: string;
        createdAt: unknown;
        user: { name: string; email: string; image?: string | null };
      }): MemberData => ({
        id: member.id,
        userId: member.userId,
        role: member.role as MemberRole,
        email: member.user.email,
        name: member.user.name,
        image: member.user.image ?? null,
        createdAt: toDate(member.createdAt),
      })
    );
  });

const updateMemberRoleServerFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { memberId: string; role: MemberRole; organizationId: string }) =>
      data
  )
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    await auth.api.updateMemberRole({
      body: {
        memberId: data.memberId,
        role: data.role,
        organizationId: data.organizationId,
      },
      headers,
    });
  });

const removeMemberServerFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { memberIdOrEmail: string; organizationId: string }) => data
  )
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    await auth.api.removeMember({
      body: {
        memberIdOrEmail: data.memberIdOrEmail,
        organizationId: data.organizationId,
      },
      headers,
    });
  });

/** Query options for fetching members of an organization. */
export const membersQueryOptions = (params: { organizationId: string }) =>
  queryOptions({
    queryKey: queryKeys.members.list(params.organizationId).queryKey,
    queryFn: () => listMembersServerFn({ data: params }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Updates a member's role within an organization. */
export const useUpdateMemberRole = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["members", "updateRole"],
    mutationFn: (data: {
      memberId: string;
      role: MemberRole;
      organizationId: string;
    }) => updateMemberRoleServerFn({ data }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.members.list(variables.organizationId).queryKey,
      });
    },
  });
};

/** Removes a member from an organization. */
export const useRemoveMember = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["members", "remove"],
    mutationFn: (data: { memberIdOrEmail: string; organizationId: string }) =>
      removeMemberServerFn({ data }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.members.list(variables.organizationId).queryKey,
      });
    },
  });
};
