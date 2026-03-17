import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { ListParams } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  createApiKeyFn,
  fetchApiKeys,
  revokeApiKeyFn,
  rotateApiKeyFn,
} from "@/lib/api";

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
