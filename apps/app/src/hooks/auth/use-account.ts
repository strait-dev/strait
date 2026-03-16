import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { authClient } from "@/lib/auth-client";

// ── Accounts (linked providers) ──────────────────────────────────────────────

type Account = {
  id: string;
  providerId: string;
  accountId: string;
};

export const useAccounts = () => {
  return useQuery({
    queryKey: ["auth", "accounts"],
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
};

export const useUnlinkAccount = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["auth", "accounts", "unlink"],
    mutationFn: async (providerId: string) => {
      const result = await authClient.unlinkAccount({ providerId });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to unlink account");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["auth", "accounts"] });
    },
  });
};

// ── Passkeys ─────────────────────────────────────────────────────────────────

type Passkey = {
  id: string;
  name: string | null;
  createdAt: string | Date | null;
};

export const usePasskeys = () => {
  return useQuery({
    queryKey: ["auth", "passkeys"],
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
};

export const useAddPasskey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["auth", "passkeys", "add"],
    mutationFn: async () => {
      const result = await authClient.passkey.addPasskey();
      if (result?.error) {
        throw new Error(result.error.message ?? "Failed to add passkey");
      }
      return result?.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["auth", "passkeys"] });
    },
  });
};

export const useDeletePasskey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["auth", "passkeys", "delete"],
    mutationFn: async (id: string) => {
      const result = await authClient.passkey.deletePasskey({ id });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to remove passkey");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["auth", "passkeys"] });
    },
  });
};

// ── Sessions ─────────────────────────────────────────────────────────────────

type Session = {
  id: string;
  token: string;
  createdAt: string | Date;
  updatedAt: string | Date;
  ipAddress: string | null;
  userAgent: string | null;
};

export const useSessions = () => {
  return useQuery({
    queryKey: ["auth", "sessions"],
    queryFn: async () => {
      const [sessionsResult, sessionResult] = await Promise.all([
        authClient.listSessions(),
        authClient.getSession(),
      ]);

      if (sessionsResult.error) {
        throw new Error(
          sessionsResult.error.message ?? "Failed to load sessions"
        );
      }

      return {
        sessions: (sessionsResult.data ?? []) as unknown as Session[],
        currentToken: sessionResult.data?.session?.token ?? null,
      };
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
};

export const useRevokeSession = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["auth", "sessions", "revoke"],
    mutationFn: async (token: string) => {
      const result = await authClient.revokeSession({ token });
      if (result.error) {
        throw new Error(result.error.message ?? "Failed to revoke session");
      }
      return result.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["auth", "sessions"] });
    },
  });
};

export const useRevokeOtherSessions = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["auth", "sessions", "revokeOthers"],
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
      queryClient.invalidateQueries({ queryKey: ["auth", "sessions"] });
    },
  });
};

export const useRevokeAllSessions = () => {
  return useMutation({
    mutationKey: ["auth", "sessions", "revokeAll"],
    mutationFn: async () => {
      const result = await authClient.revokeSessions();
      if (result.error) {
        throw new Error(
          result.error.message ?? "Failed to sign out of all sessions"
        );
      }
      return result.data;
    },
  });
};
