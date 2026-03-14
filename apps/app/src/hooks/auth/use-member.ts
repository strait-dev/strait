import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";

type MemberData = {
  id: string;
  userId: string;
  organizationId: string;
  role: string;
  createdAt: Date;
  user?: {
    id: string;
    name: string;
    email: string;
    image?: string | null;
  };
};

/** API response format for members list. */
export type MembersApiResponse = {
  page: MemberData[];
  pageCount: number;
};

/** Parameters for member queries. */
interface MemberParams {
  organizationId: string;
}

const toDate = (value: unknown) =>
  value instanceof Date ? value : new Date(String(value));

const mapMember = (member: {
  id: string;
  userId: string;
  organizationId: string;
  role: string;
  createdAt: unknown;
  user?: {
    id: string;
    name?: string | null;
    email: string;
    image?: string | null;
  } | null;
}): MemberData => ({
  id: member.id,
  userId: member.userId,
  organizationId: member.organizationId,
  role: member.role,
  createdAt: toDate(member.createdAt),
  user: member.user
    ? {
        id: member.user.id,
        name: member.user.name ?? "",
        email: member.user.email,
        image: member.user.image ?? null,
      }
    : undefined,
});

const listMembersServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: MemberParams) => data)
  .handler(async ({ data }) => {
    const { auth } = await import("@/lib/auth");
    const headers = getRequestHeaders();
    const result = await auth.api.listMembers({
      query: { organizationId: data.organizationId },
      headers,
    });

    return (result.members ?? []).map((member) => mapMember(member));
  });

const updateMemberRoleServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { memberId: string; role: string }) => data)
  .handler(async ({ data }) => {
    const { auth } = await import("@/lib/auth");
    const headers = getRequestHeaders();
    const member = await auth.api.updateMemberRole({
      body: {
        memberId: data.memberId,
        role: data.role,
      },
      headers,
    });

    return mapMember(member);
  });

const removeMemberServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { memberIdOrEmail: string }) => data)
  .handler(async ({ data }) => {
    const { auth } = await import("@/lib/auth");
    const headers = getRequestHeaders();
    await auth.api.removeMember({
      body: {
        memberIdOrEmail: data.memberIdOrEmail,
      },
      headers,
    });
  });

/** Query options for fetching members in an organization. */
export const membersQueryOptions = (params: MemberParams) =>
  queryOptions({
    queryKey: ["members", params.organizationId],
    queryFn: () => listMembersServerFn({ data: params }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Parameters for member detail queries. */
interface MemberDetailParams {
  id: string;
}

/** Query options for fetching a single member. */
export const memberQueryOptions = (params: MemberDetailParams) =>
  queryOptions({
    queryKey: ["members", "detail", params.id],
    queryFn: () =>
      listMembersServerFn({ data: { organizationId: params.id } }).then(
        (members) => members[0] ?? null
      ),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Updates a member's role. */
const useUpdateMemberRole = () => {
  return useMutation({
    mutationKey: ["members", "update"],
    mutationFn: (data: { memberId: string; role: string }) =>
      updateMemberRoleServerFn({ data }),
  });
};

/** Removes a member from an organization. */
const useRemoveMember = () => {
  return useMutation({
    mutationKey: ["members", "bulkDelete"],
    mutationFn: (data: { memberIdOrEmail: string } | string) =>
      removeMemberServerFn({
        data:
          typeof data === "string"
            ? { memberIdOrEmail: data }
            : { memberIdOrEmail: data.memberIdOrEmail },
      }),
  });
};

export const useUpdateMember = useUpdateMemberRole;
export const useDeleteMembers = useRemoveMember;

export type { MemberParams };
