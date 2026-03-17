import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { ListParams, WebhookSubscription } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  createWebhookFn,
  deleteWebhookFn,
  fetchWebhookDeliveries,
  fetchWebhookSubscriptions,
  testWebhookFn,
} from "@/lib/api";

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
