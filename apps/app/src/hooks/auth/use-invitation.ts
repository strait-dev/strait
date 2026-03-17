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

export type InvitationData = {
  id: string;
  email: string;
  role: string;
  status: "pending" | "accepted" | "cancelled" | "expired" | "rejected";
  organizationId: string;
  inviterId: string;
  expiresAt: Date;
};

type InvitationRole = "member" | "admin" | "owner";

/** API response format for invitations list. */
export type InvitationsApiResponse = {
  page: InvitationData[];
  pageCount: number;
};

/** Parameters for invitation queries. */
interface InvitationParams {
  organizationId: string;
}

/** Parameters for invitation ID queries. */
interface InvitationIdParams {
  invitationId: string;
}

const toDate = (value: unknown) =>
  value instanceof Date ? value : new Date(String(value));

const mapInvitation = (invitation: {
  id: string;
  email: string;
  role: string;
  status:
    | "pending"
    | "accepted"
    | "cancelled"
    | "canceled"
    | "expired"
    | "rejected";
  organizationId: string;
  inviterId: string;
  expiresAt: unknown;
}): InvitationData => ({
  id: invitation.id,
  email: invitation.email,
  role: invitation.role,
  status: invitation.status === "canceled" ? "cancelled" : invitation.status,
  organizationId: invitation.organizationId,
  inviterId: invitation.inviterId,
  expiresAt: toDate(invitation.expiresAt),
});

const listInvitationsServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: InvitationParams) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const invitations = await auth.api.listInvitations({
      query: { organizationId: data.organizationId },
      headers,
    });

    return (invitations ?? []).map((invitation: Parameters<typeof mapInvitation>[0]) => mapInvitation(invitation));
  });

const getInvitationServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const invitation = await auth.api.getInvitation({
      query: { id: data.id },
      headers,
    });

    if (!invitation) {
      return null;
    }

    return mapInvitation(invitation);
  });

const createInvitationServerFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { email: string; role: InvitationRole; organizationId: string }) =>
      data
  )
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const invitation = await auth.api.createInvitation({
      body: {
        email: data.email,
        role: data.role,
        organizationId: data.organizationId,
      },
      headers,
    });

    return mapInvitation(invitation);
  });

const cancelInvitationServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { invitationId: string }) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    await auth.api.cancelInvitation({
      body: { invitationId: data.invitationId },
      headers,
    });
  });

/** Query options for fetching invitations in an organization. */
export const invitationsQueryOptions = (params: InvitationParams) =>
  queryOptions({
    queryKey: ["invitations", params.organizationId],
    queryFn: () => listInvitationsServerFn({ data: params }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Query options for fetching a single invitation. */
export const invitationQueryOptions = (params: InvitationIdParams) =>
  queryOptions({
    queryKey: ["invitations", params.invitationId],
    queryFn: () => getInvitationServerFn({ data: { id: params.invitationId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Represents public invitation data for unauthenticated users. */
export type PublicInvitationData = {
  id: string;
  invitationId: string;
  email: string;
  status: "pending" | "accepted" | "cancelled" | "expired" | "rejected";
  expiresAt: Date;
  role?: string;
  organization?: { name: string };
  inviter?: { name?: string; email: string };
};

export const getPublicInvitationServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .handler(async ({ data }): Promise<PublicInvitationData | null> => {
    const headers = getRequestHeaders();
    const invitation = await auth.api.getInvitation({
      query: { id: data.id },
      headers,
    });

    if (!invitation) {
      return null;
    }

    return {
      id: invitation.id,
      invitationId: invitation.id,
      email: invitation.email,
      status:
        invitation.status === "canceled" ? "cancelled" : invitation.status,
      expiresAt: toDate(invitation.expiresAt),
      role: invitation.role,
      organization: undefined,
      inviter: undefined,
    };
  });

/** Creates a new invitation. */
export const useCreateInvitation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["invitations", "create"],
    mutationFn: (data: {
      email: string;
      role: InvitationRole;
      organizationId: string;
    }) => createInvitationServerFn({ data }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.invitations.list(variables.organizationId).queryKey,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.members._def,
      });
    },
  });
};

/** Cancels an invitation. */
export const useCancelInvitation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["invitations", "cancel"],
    mutationFn: (
      data: { invitationId: string; organizationId?: string } | string
    ) =>
      cancelInvitationServerFn({
        data:
          typeof data === "string"
            ? { invitationId: data }
            : { invitationId: data.invitationId },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.invitations._def,
      });
    },
  });
};

/** Updates (cancels) an invitation. */
export const useUpdateInvitation = () => {
  return useMutation({
    mutationKey: ["invitations", "cancel"],
    mutationFn: (data: { invitationId: string } | string) =>
      cancelInvitationServerFn({
        data:
          typeof data === "string"
            ? { invitationId: data }
            : { invitationId: data.invitationId },
      }),
  });
};

/** Deletes (cancels) invitations. */
export const useDeleteInvitations = () => {
  return useMutation({
    mutationKey: ["invitations", "cancel"],
    mutationFn: (data: { invitationId: string } | string) =>
      cancelInvitationServerFn({
        data:
          typeof data === "string"
            ? { invitationId: data }
            : { invitationId: data.invitationId },
      }),
  });
};

export type UserInvitationData = InvitationData & {
  organizationName?: string;
  inviterName?: string;
};

const listUserInvitationsServerFn = createServerFn({ method: "GET" }).handler(
  async () => {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    if (!session?.user?.email) {
      return [];
    }

    const orgs = await auth.api.listOrganizations({ headers });
    const allInvitations: UserInvitationData[] = [];

    for (const org of orgs ?? []) {
      try {
        const invitations = await auth.api.listInvitations({
          query: { organizationId: org.id },
          headers,
        });
        for (const inv of invitations ?? []) {
          if (inv.email === session.user.email && inv.status === "pending") {
            allInvitations.push({
              ...mapInvitation(inv),
              organizationName: org.name,
            });
          }
        }
      } catch {
        // Skip orgs where we can't list invitations
      }
    }

    return allInvitations;
  }
);

/** Query options for fetching invitations sent to the current user. */
export const userInvitationsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.userInvitations.list.queryKey,
    queryFn: () => listUserInvitationsServerFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Accepts an invitation (client-side via authClient). */
export const useAcceptInvitation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["invitations", "accept"],
    mutationFn: async (invitationId: string) => {
      // This needs to be called client-side; we import dynamically
      const { authClient } = await import("@/lib/auth-client");
      const result = await authClient.organization.acceptInvitation({
        invitationId,
      });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to accept invitation");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.userInvitations._def,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.organizations._def,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.members._def,
      });
    },
  });
};

/** Rejects an invitation (client-side via authClient). */
export const useRejectInvitation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["invitations", "reject"],
    mutationFn: async (invitationId: string) => {
      const { authClient } = await import("@/lib/auth-client");
      const result = await authClient.organization.rejectInvitation({
        invitationId,
      });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to reject invitation");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.userInvitations._def,
      });
    },
  });
};

export type { InvitationIdParams, InvitationParams };
