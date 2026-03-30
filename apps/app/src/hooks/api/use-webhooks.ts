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
import { getPostHog } from "@/lib/analytics";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export const fetchWebhookSubscriptions = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(
    async ({ data }): Promise<PaginatedResponse<WebhookSubscription>> => {
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<WebhookSubscription>>(
          "/v1/webhooks/subscriptions",
          { params: { limit: data.limit, cursor: data.cursor } }
        )
      );
    }
  );

export const fetchWebhookDeliveries = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<PaginatedResponse<WebhookDelivery>> => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<WebhookDelivery>>("/v1/webhooks/deliveries", {
        params: { limit: data.limit, cursor: data.cursor },
      })
    );
  });

export const createWebhookFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { webhook_url: string; event_types: string[] }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<WebhookSubscription> => {
    return await runWithSentryReport(
      apiEffect<WebhookSubscription>("/v1/webhooks/subscriptions", {
        method: "POST",
        body: data,
      })
    );
  });

export const deleteWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/webhooks/subscriptions/${data.id}`, {
        method: "DELETE",
      })
    );
  });

export const testWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { url: string; secret?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<WebhookDelivery> => {
    return await runWithSentryReport(
      apiEffect<WebhookDelivery>("/v1/webhooks/test", {
        method: "POST",
        body: data,
      })
    );
  });

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
    enabled: !!webhookId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useCreateWebhook = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["webhooks", "create"],
    mutationFn: (data: { webhook_url: string; event_types: string[] }) =>
      createWebhookFn({ data }),
    onSuccess: (data) => {
      getPostHog()?.capture("webhook_created", { webhook_id: data?.id });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "webhook_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
    },
  });
};

export const useDeleteWebhook = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["webhooks", "delete"],
    mutationFn: (id: string) => deleteWebhookFn({ data: { id } }),
    onSuccess: (_data, id) => {
      getPostHog()?.capture("webhook_deleted", { webhook_id: id });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "webhook_deleted",
        error_message: err instanceof Error ? err.message : "Unknown error",
        webhook_id: variables,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
    },
  });
};

export const useTestWebhook = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["webhooks", "test"],
    mutationFn: (webhookUrl: string) =>
      testWebhookFn({ data: { url: webhookUrl } }),
    onSuccess: () => {
      getPostHog()?.capture("webhook_tested");
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "webhook_tested",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.webhooks.deliveries._def,
      });
    },
  });
};
