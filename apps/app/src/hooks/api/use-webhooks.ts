import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  ListParams,
  PaginatedResponse,
  WebhookDelivery,
  WebhookSubscription,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiRequest } from "@/lib/api-client.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchWebhookSubscriptions = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<PaginatedResponse<WebhookSubscription>>(
      "/v1/webhooks/subscriptions",
      { params: { limit: data.limit, cursor: data.cursor } }
    );
  });

export const fetchWebhookDeliveries = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<PaginatedResponse<WebhookDelivery>>(
      "/v1/webhooks/deliveries",
      { params: { limit: data.limit, cursor: data.cursor } }
    );
  });

export const createWebhookFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { webhook_url: string; event_types: string[] }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<WebhookSubscription>("/v1/webhooks/subscriptions", {
      method: "POST",
      body: data,
    });
  });

export const deleteWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<void>(`/v1/webhooks/subscriptions/${data.id}`, {
      method: "DELETE",
    });
  });

export const testWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { webhook_url: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<WebhookDelivery>("/v1/webhooks/test", {
      method: "POST",
      body: data,
    });
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const webhooksQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.webhooks.list(search).queryKey,
    queryFn: () => fetchWebhookSubscriptions({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const webhookQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.webhooks.detail(id).queryKey,
    queryFn: () =>
      fetchWebhookSubscriptions({ data: {} }).then((res) => {
        const found = res.data.find((w) => w.id === id);
        if (!found) {
          throw new Error(`Webhook not found: ${id}`);
        }
        return found;
      }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const webhookDeliveriesQueryOptions = (webhookId: string) =>
  queryOptions({
    queryKey: queryKeys.webhooks.deliveries(webhookId).queryKey,
    queryFn: () => fetchWebhookDeliveries({ data: {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useCreateWebhook = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["webhooks", "create"],
    mutationFn: (
      data: Pick<WebhookSubscription, "webhook_url" | "event_types">
    ) => createWebhookFn({ data }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
    },
  });
};

export const useDeleteWebhook = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["webhooks", "delete"],
    mutationFn: (id: string) => deleteWebhookFn({ data: { id } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
    },
  });
};

export const useTestWebhook = () =>
  useMutation({
    mutationKey: ["webhooks", "test"],
    mutationFn: (webhookUrl: string) =>
      testWebhookFn({ data: { webhook_url: webhookUrl } }),
  });
