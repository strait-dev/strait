import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getAuth } from "@/lib/auth.server";
import { authClient } from "@/lib/auth-client";
import { authMiddleware } from "@/middlewares/auth";

type Account = {
  id: string;
  providerId: string;
  accountId: string;
};

type Passkey = {
  id: string;
  name: string | null;
  createdAt: string | Date | null;
};

type Session = {
  id: string;
  createdAt: string | Date;
  updatedAt: string | Date;
  ipAddress: string | null;
  userAgent: string | null;
  isCurrent: boolean;
};

type RawAuthSession = Omit<Session, "isCurrent"> & {
  token: string;
};

const listSessionsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const headers = getRequestHeaders();
    const [sessions, currentSession] = await Promise.all([
      (await getAuth()).api.listSessions({ headers }),
      (await getAuth()).api.getSession({ headers }),
    ]);
    const currentToken = currentSession?.session?.token ?? null;
    return ((sessions ?? []) as unknown as RawAuthSession[]).map(
      ({ token, ...session }) => ({
        ...session,
        isCurrent: token === currentToken,
      })
    );
  });

const revokeSessionServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { sessionId: string }) =>
    z.object({ sessionId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const sessions = ((await (
      await getAuth()
    ).api.listSessions({
      headers,
    })) ?? []) as unknown as RawAuthSession[];
    const session = sessions.find((item) => item.id === data.sessionId);
    if (!session) {
      throw new Error("Session not found");
    }
    await (await getAuth()).api.revokeSession({
      body: { token: session.token },
      headers,
    });
  });

/** Query options for fetching linked authentication accounts (Google, GitHub, credential). */
export const accountsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.auth.accounts.queryKey,
    queryFn: async () => {
      const result = await authClient.listAccounts();
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to load accounts");
      }
      return (result.data ?? []) as unknown as Account[];
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Query options for fetching the user's registered WebAuthn passkeys. */
export const passkeysQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.auth.passkeys.queryKey,
    queryFn: async () => {
      const result = await authClient.passkey.listUserPasskeys();
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to load passkeys");
      }
      return (result.data ?? []) as unknown as Passkey[];
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Query options for fetching all active sessions and identifying the current one. */
export const sessionsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.auth.sessions.queryKey,
    queryFn: async () => ({ sessions: await listSessionsServerFn() }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Unlinks a social provider from the current user's account. */
export const useUnlinkAccount = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (providerId: string) => {
      const result = await authClient.unlinkAccount({ providerId });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to unlink account");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.accounts.queryKey,
      });
    },
  });
};

/** Registers a new WebAuthn passkey for the current user. */
export const useAddPasskey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const result = await authClient.passkey.addPasskey();
      if (result?.error) {
        throw new Error(
          String(result.error.message ?? "Failed to add passkey")
        );
      }
      return result?.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.passkeys.queryKey,
      });
    },
  });
};

/** Deletes a WebAuthn passkey by ID. */
export const useDeletePasskey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const result = await authClient.passkey.deletePasskey({ id });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to remove passkey");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.passkeys.queryKey,
      });
    },
  });
};

/** Revokes a specific session by its token. */
export const useRevokeSession = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (sessionId: string) =>
      revokeSessionServerFn({ data: { sessionId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.sessions.queryKey,
      });
    },
  });
};

/** Revokes all sessions except the current one. */
export const useRevokeOtherSessions = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const result = await authClient.revokeOtherSessions();
      if (result.error) {
        throw new Error(
          result.error.message ?? "Failed to revoke other sessions"
        );
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.sessions.queryKey,
      });
    },
  });
};

/** Revokes all sessions including the current one (sign out everywhere). */
export const useRevokeAllSessions = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const result = await authClient.revokeSessions();
      if (result.error) {
        throw new Error(
          result.error.message ?? "Failed to sign out of all sessions"
        );
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.sessions.queryKey,
      });
    },
  });
};
