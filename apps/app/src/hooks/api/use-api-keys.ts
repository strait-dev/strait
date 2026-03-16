import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { APIKey, ListParams } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";

type ListApiKeysSearch = ListParams;

const now = new Date().toISOString();
const oneMonthAgo = new Date(Date.now() - 30 * 86_400_000).toISOString();
const twoMonthsAgo = new Date(Date.now() - 60 * 86_400_000).toISOString();
const yesterday = new Date(Date.now() - 86_400_000).toISOString();

const MOCK_API_KEYS: APIKey[] = [
  {
    id: "key_01",
    project_id: "proj_01",
    name: "Production API Key",
    key_prefix: "strait_a1b2c3",
    scopes: ["jobs:read", "jobs:write", "jobs:trigger"],
    expires_at: null,
    last_used_at: yesterday,
    created_at: twoMonthsAgo,
    revoked_at: null,
    replaced_by_key_id: "",
    grace_expires_at: null,
  },
  {
    id: "key_02",
    project_id: "proj_01",
    name: "Development Key",
    key_prefix: "strait_d4e5f6",
    scopes: ["jobs:read"],
    expires_at: null,
    last_used_at: now,
    created_at: oneMonthAgo,
    revoked_at: null,
    replaced_by_key_id: "",
    grace_expires_at: null,
  },
  {
    id: "key_03",
    project_id: "proj_01",
    name: "CI/CD Pipeline",
    key_prefix: "strait_g7h8i9",
    scopes: ["jobs:read", "jobs:write", "jobs:trigger", "api_keys:manage"],
    expires_at: null,
    last_used_at: null,
    created_at: now,
    revoked_at: null,
    replaced_by_key_id: "",
    grace_expires_at: null,
  },
];

function mockDelay() {
  return new Promise<void>((resolve) => setTimeout(resolve, 300));
}

/** Query options for listing API keys. */
export const apiKeysQueryOptions = (search?: ListApiKeysSearch) =>
  queryOptions({
    queryKey: queryKeys.apiKeys.list(search).queryKey,
    queryFn: async () => {
      await mockDelay();
      return {
        data: MOCK_API_KEYS.filter((k) => !k.revoked_at),
        page_count: 1,
        total_count: MOCK_API_KEYS.filter((k) => !k.revoked_at).length,
      };
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Creates a new API key. Returns the full key only once. */
export const useCreateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "create"],
    mutationFn: async (data: { name: string; scopes: string[] }) => {
      await mockDelay();
      const newKey: APIKey & { key: string } = {
        id: `key_${Date.now()}`,
        project_id: "proj_01",
        name: data.name,
        key_prefix: `strait_${Math.random().toString(36).slice(2, 8)}`,
        key: `strait_${Array.from({ length: 64 }, () => Math.random().toString(36)[2]).join("")}`,
        scopes: data.scopes,
        expires_at: null,
        last_used_at: null,
        created_at: new Date().toISOString(),
        revoked_at: null,
        replaced_by_key_id: "",
        grace_expires_at: null,
      };
      return newKey;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};

/** Revokes an API key. */
export const useRevokeApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "revoke"],
    mutationFn: async (keyId: string) => {
      await mockDelay();
      return { id: keyId, status: "revoked" };
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};
