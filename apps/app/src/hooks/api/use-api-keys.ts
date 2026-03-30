import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  APIKey,
  ListParams,
  PaginatedResponse,
  RotateAPIKeyResponse,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export const fetchApiKeys = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<APIKey>>("/v1/api-keys", {
        params: { limit: data.limit, cursor: data.cursor },
      })
    );
  });

export const createApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { name: string; scopes: string[]; expiresInDays?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<APIKey & { key: string }>("/v1/api-keys", {
        method: "POST",
        body: {
          name: data.name,
          scopes: data.scopes,
          expires_in_days: data.expiresInDays,
        },
      })
    );
  });

export const revokeApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/api-keys/${data.keyId}`, {
        method: "DELETE",
      })
    );
  });

export const rotateApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { keyId: string; gracePeriodMinutes?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<RotateAPIKeyResponse>(`/v1/api-keys/${data.keyId}/rotate`, {
        method: "POST",
        body: data.gracePeriodMinutes
          ? { grace_period_minutes: data.gracePeriodMinutes }
          : undefined,
      })
    );
  });

export const apiKeysQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.apiKeys.list(search).queryKey,
    queryFn: () => fetchApiKeys({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const useCreateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "create"],
    mutationFn: (data: {
      name: string;
      scopes: string[];
      expiresInDays?: number;
    }) => createApiKeyFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("api_key_created", {
        key_name: variables.name,
        scopes: variables.scopes,
      });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "api_key_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
        key_name: variables.name,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};

export const useRevokeApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "revoke"],
    mutationFn: (keyId: string) => revokeApiKeyFn({ data: { keyId } }),
    onSuccess: (_data, keyId) => {
      getPostHog()?.capture("api_key_revoked", { key_id: keyId });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "api_key_revoked",
        error_message: err instanceof Error ? err.message : "Unknown error",
        key_id: variables,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};

export const useRotateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["apiKeys", "rotate"],
    mutationFn: (keyId: string) => rotateApiKeyFn({ data: { keyId } }),
    onSuccess: (_data, keyId) => {
      getPostHog()?.capture("api_key_rotated", { key_id: keyId });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "api_key_rotated",
        error_message: err instanceof Error ? err.message : "Unknown error",
        key_id: variables,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys._def });
    },
  });
};
