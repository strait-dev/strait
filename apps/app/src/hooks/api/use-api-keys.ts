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
import { getPostHog } from "@/lib/analytics";
import { apiPath } from "@/lib/api-client.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

const allowedApiKeyScopes = new Set([
  "jobs:read",
  "jobs:write",
  "jobs:trigger",
  "runs:read",
  "workflows:read",
  "workflows:write",
  "webhooks:read",
  "webhooks:write",
  "api-keys:manage",
]);

function validateApiKeyScopes(scopes: string[]): void {
  if (scopes.length === 0) {
    throw new Error("At least one scope is required");
  }
  for (const scope of scopes) {
    if (!allowedApiKeyScopes.has(scope)) {
      throw new Error("Unsupported API key scope");
    }
  }
}

const fetchApiKeys = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<APIKey>>("/v1/api-keys", {
        params: { limit: data.limit, cursor: data.cursor },
      })
    );
  });

const createApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { name: string; scopes: string[]; expiresInDays?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const projectId = await requireActiveProjectAdmin(context);
    validateApiKeyScopes(data.scopes);
    return await runWithSentryReport(
      apiEffect<APIKey & { key: string }>("/v1/api-keys", {
        method: "POST",
        body: {
          name: data.name,
          project_id: projectId,
          scopes: data.scopes,
          expires_in_days: data.expiresInDays,
        },
      })
    );
  });

const revokeApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<void>(apiPath`/v1/api-keys/${data.keyId}`, {
        method: "DELETE",
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
