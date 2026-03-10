import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { auth } from "@/lib/auth";

type InvitationData = {
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

    return (invitations ?? []).map((invitation) => mapInvitation(invitation));
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
  return useMutation({
    mutationKey: ["invitations", "create"],
    mutationFn: (data: {
      email: string;
      role: InvitationRole;
      organizationId: string;
    }) => createInvitationServerFn({ data }),
  });
};

/** Cancels an invitation. */
export const useCancelInvitation = () => {
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

export type { InvitationParams, InvitationIdParams };
