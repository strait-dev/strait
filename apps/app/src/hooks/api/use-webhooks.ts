import { toast } from "@strait/ui/components/toast";
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
import { apiPath } from "@/lib/api-client.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

export type CreateWebhookResult = {
  subscription: WebhookSubscription;
  signing_secret: string;
};

type WebhookSubscriptionsResponse =
  | PaginatedResponse<WebhookSubscription>
  | WebhookSubscription[]
  | null;

/** Normalize the legacy array response and the cursor envelope shape. */
function normalizeWebhookSubscriptions(
  response: WebhookSubscriptionsResponse
): PaginatedResponse<WebhookSubscription> {
  if (Array.isArray(response)) {
    return { data: response, has_more: false };
  }

  return {
    data: response?.data ?? [],
    has_more: response?.has_more ?? false,
    next_cursor: response?.next_cursor,
  };
}

export const fetchWebhookSubscriptions = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(
    async ({
      context,
      data,
    }): Promise<PaginatedResponse<WebhookSubscription>> => {
      await requireActiveProjectAccess(context);
      const response = await runWithSentryReport(
        apiEffect<WebhookSubscriptionsResponse>("/v1/webhooks/subscriptions", {
          params: { limit: data.limit, cursor: data.cursor },
        })
      );
      return normalizeWebhookSubscriptions(response);
    }
  );

export const fetchWebhookDeliveries = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams & { webhookId?: string }) => data)
  .middleware([authMiddleware])
  .handler(
    async ({ context, data }): Promise<PaginatedResponse<WebhookDelivery>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<WebhookDelivery>>(
          "/v1/webhooks/deliveries",
          {
            params: {
              limit: data.limit,
              cursor: data.cursor,
              webhook_id: data.webhookId,
            },
          }
        )
      );
    }
  );

export const createWebhookFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { webhook_url: string; event_types: string[] }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<CreateWebhookResult> => {
    const projectId = await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<CreateWebhookResult>("/v1/webhooks/subscriptions", {
        method: "POST",
        body: {
          project_id: projectId,
          webhook_url: data.webhook_url,
          event_types: data.event_types,
        },
      })
    );
  });

export const deleteWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<void> => {
    await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<void>(apiPath`/v1/webhooks/subscriptions/${data.id}`, {
        method: "DELETE",
      })
    );
  });

export const testWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { url: string; secret?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<WebhookDelivery> => {
    await requireActiveProjectAdmin(context);
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
    queryFn: () => fetchWebhookDeliveries({ data: { webhookId } }),
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
      getPostHog()?.capture("webhook_created", {
        webhook_id: data.subscription.id,
      });
    },
    onError: (err) => {
      toast.error("Failed to create webhook.");
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
    onMutate: async (id) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.webhooks.list._def,
      });

      const previousLists = queryClient.getQueriesData<
        PaginatedResponse<WebhookSubscription>
      >({ queryKey: queryKeys.webhooks.list._def });

      queryClient.setQueriesData<PaginatedResponse<WebhookSubscription>>(
        { queryKey: queryKeys.webhooks.list._def },
        (old) =>
          old ? { ...old, data: old.data.filter((w) => w.id !== id) } : old
      );

      return { previousLists };
    },
    onSuccess: (_data, id) => {
      getPostHog()?.capture("webhook_deleted", { webhook_id: id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to delete webhook.");
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          queryClient.setQueryData(key, data);
        }
      }
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
      toast.error("Failed to test webhook.");
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
