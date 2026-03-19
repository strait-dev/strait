import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type { APIKey, ListParams, PaginatedResponse } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiRequest } from "@/lib/api-client.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchApiKeys = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<PaginatedResponse<APIKey>>("/v1/api-keys", {
      params: { limit: data.limit, cursor: data.cursor },
    });
  });

export const createApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { name: string; scopes: string[]; expiresInDays?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const expiresIn = data.expiresInDays
      ? `${data.expiresInDays * 24}h`
      : undefined;
    return await apiRequest<APIKey & { key: string }>("/v1/api-keys", {
      method: "POST",
      body: { name: data.name, scopes: data.scopes, expires_in: expiresIn },
    });
  });

export const revokeApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<void>(`/v1/api-keys/${data.keyId}`, {
      method: "DELETE",
    });
  });

export const rotateApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<APIKey & { key: string }>(
      `/v1/api-keys/${data.keyId}/rotate`,
      { method: "POST" }
    );
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const apiKeysQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.apiKeys.list(search).queryKey,
    queryFn: () => fetchApiKeys({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useCreateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "create"],
    mutationFn: (data: {
      name: string;
      scopes: string[];
      expiresInDays?: number;
    }) => createApiKeyFn({ data }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};

export const useRevokeApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "revoke"],
    mutationFn: (keyId: string) => revokeApiKeyFn({ data: { keyId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};

export const useRotateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "rotate"],
    mutationFn: (keyId: string) => rotateApiKeyFn({ data: { keyId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};
