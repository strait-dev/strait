import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import type {
  ListParams,
  PaginatedResponse,
  WebhookDelivery,
  WebhookSubscription,
} from "@/hooks/api/types.ts";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const MOCK_WEBHOOKS: WebhookSubscription[] = [
  {
    id: "wh_001",
    project_id: "proj_001",
    webhook_url: "https://api.example.com/webhooks/runs",
    event_types: ["run.completed", "run.failed"],
    secret: "whsec_abc123",
    active: true,
    created_at: "2026-02-10T12:00:00Z",
  },
  {
    id: "wh_002",
    project_id: "proj_001",
    webhook_url: "https://hooks.slack.com/services/T00/B00/xxx",
    event_types: ["run.failed", "run.timed_out"],
    secret: "whsec_def456",
    active: true,
    created_at: "2026-02-15T09:30:00Z",
  },
  {
    id: "wh_003",
    project_id: "proj_001",
    webhook_url: "https://internal.corp.net/orchestrator/events",
    event_types: [
      "run.completed",
      "run.failed",
      "workflow.completed",
      "workflow.failed",
    ],
    secret: "whsec_ghi789",
    active: true,
    created_at: "2026-02-20T14:00:00Z",
  },
  {
    id: "wh_004",
    project_id: "proj_001",
    webhook_url: "https://old-service.example.com/notify",
    event_types: ["run.completed"],
    secret: "whsec_jkl012",
    active: false,
    created_at: "2026-01-05T08:00:00Z",
  },
];

const MOCK_DELIVERIES: WebhookDelivery[] = [
  {
    id: "del_001",
    run_id: "run_a1",
    job_id: "job_001",
    event_trigger_id: "evt_001",
    webhook_url: "https://api.example.com/webhooks/runs",
    webhook_retry_policy: "exponential",
    status: "delivered",
    attempts: 1,
    max_attempts: 3,
    last_status_code: 200,
    last_error: "",
    next_retry_at: null,
    delivered_at: "2026-03-14T08:00:05Z",
    created_at: "2026-03-14T08:00:02Z",
    updated_at: "2026-03-14T08:00:05Z",
  },
  {
    id: "del_002",
    run_id: "run_a2",
    job_id: "job_001",
    event_trigger_id: "evt_004",
    webhook_url: "https://api.example.com/webhooks/runs",
    webhook_retry_policy: "exponential",
    status: "delivered",
    attempts: 2,
    max_attempts: 3,
    last_status_code: 200,
    last_error: "",
    next_retry_at: null,
    delivered_at: "2026-03-14T08:01:15Z",
    created_at: "2026-03-14T08:01:02Z",
    updated_at: "2026-03-14T08:01:15Z",
  },
  {
    id: "del_003",
    run_id: "run_a3",
    job_id: "job_002",
    event_trigger_id: "evt_006",
    webhook_url: "https://hooks.slack.com/services/T00/B00/xxx",
    webhook_retry_policy: "fixed",
    status: "failed",
    attempts: 3,
    max_attempts: 3,
    last_status_code: 502,
    last_error: "Bad Gateway",
    next_retry_at: null,
    delivered_at: null,
    created_at: "2026-03-14T08:02:02Z",
    updated_at: "2026-03-14T08:02:30Z",
  },
  {
    id: "del_004",
    run_id: "run_a4",
    job_id: "job_003",
    event_trigger_id: "evt_008",
    webhook_url: "https://internal.corp.net/orchestrator/events",
    webhook_retry_policy: "exponential",
    status: "pending",
    attempts: 1,
    max_attempts: 3,
    last_status_code: 500,
    last_error: "Internal Server Error",
    next_retry_at: "2026-03-14T08:05:00Z",
    delivered_at: null,
    created_at: "2026-03-14T08:03:02Z",
    updated_at: "2026-03-14T08:03:10Z",
  },
  {
    id: "del_005",
    run_id: "run_a5",
    job_id: "job_001",
    event_trigger_id: "evt_010",
    webhook_url: "https://internal.corp.net/orchestrator/events",
    webhook_retry_policy: "exponential",
    status: "delivered",
    attempts: 1,
    max_attempts: 3,
    last_status_code: 200,
    last_error: "",
    next_retry_at: null,
    delivered_at: "2026-03-14T08:04:03Z",
    created_at: "2026-03-14T08:04:01Z",
    updated_at: "2026-03-14T08:04:03Z",
  },
];

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

function filterWebhooks(
  webhooks: WebhookSubscription[],
  search?: ListParams
): PaginatedResponse<WebhookSubscription> {
  let filtered = webhooks;

  if (search?.query) {
    const q = search.query.toLowerCase();
    filtered = filtered.filter((w) => w.webhook_url.toLowerCase().includes(q));
  }

  const perPage = search?.per_page ?? 20;
  const page = search?.page ?? 1;
  const start = (page - 1) * perPage;
  const paged = filtered.slice(start, start + perPage);

  return {
    data: paged,
    total_count: filtered.length,
    page_count: Math.ceil(filtered.length / perPage),
  };
}

// ---------------------------------------------------------------------------
// Exports
// ---------------------------------------------------------------------------

/** Query options for listing webhook subscriptions. */
export const webhooksQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: ["webhooks", search ?? {}],
    queryFn: () => Promise.resolve(filterWebhooks(MOCK_WEBHOOKS, search)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Query options for a single webhook subscription by ID. */
export const webhookQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ["webhooks", id],
    queryFn: () => {
      const webhook = MOCK_WEBHOOKS.find((w) => w.id === id);
      if (!webhook) {
        throw new Error(`Webhook not found: ${id}`);
      }
      return Promise.resolve(webhook);
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Query options for deliveries belonging to a specific webhook. */
export const webhookDeliveriesQueryOptions = (webhookId: string) =>
  queryOptions({
    queryKey: ["webhooks", webhookId, "deliveries"],
    queryFn: () => {
      const webhook = MOCK_WEBHOOKS.find((w) => w.id === webhookId);
      if (!webhook) {
        return Promise.resolve({ data: [], total_count: 0, page_count: 0 });
      }
      // Filter deliveries by matching webhook_url
      const deliveries = MOCK_DELIVERIES.filter(
        (d) => d.webhook_url === webhook.webhook_url
      );
      return Promise.resolve({
        data: deliveries,
        total_count: deliveries.length,
        page_count: 1,
      } satisfies PaginatedResponse<WebhookDelivery>);
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Mutation to create a new webhook subscription. */
export const useCreateWebhook = () =>
  useMutation({
    mutationKey: ["webhooks", "create"],
    mutationFn: (
      data: Pick<WebhookSubscription, "webhook_url" | "event_types">
    ) =>
      Promise.resolve({
        id: `wh_${Date.now().toString(36)}`,
        project_id: "proj_001",
        webhook_url: data.webhook_url,
        event_types: data.event_types,
        secret: `whsec_${Date.now().toString(36)}`,
        active: true,
        created_at: new Date().toISOString(),
      } satisfies WebhookSubscription),
  });

/** Mutation to delete a webhook subscription by ID. */
export const useDeleteWebhook = () =>
  useMutation({
    mutationKey: ["webhooks", "delete"],
    mutationFn: (id: string) => {
      const exists = MOCK_WEBHOOKS.some((w) => w.id === id);
      if (!exists) {
        return Promise.reject(new Error(`Webhook not found: ${id}`));
      }
      return Promise.resolve({ success: true });
    },
  });

/** Mutation to send a test delivery for a webhook subscription. */
export const useTestWebhook = () =>
  useMutation({
    mutationKey: ["webhooks", "test"],
    mutationFn: (id: string) => {
      const webhook = MOCK_WEBHOOKS.find((w) => w.id === id);
      if (!webhook) {
        return Promise.reject(new Error(`Webhook not found: ${id}`));
      }
      return Promise.resolve({
        id: `del_test_${Date.now().toString(36)}`,
        run_id: "",
        job_id: "",
        event_trigger_id: "",
        webhook_url: webhook.webhook_url,
        webhook_retry_policy: "none",
        status: "delivered",
        attempts: 1,
        max_attempts: 1,
        last_status_code: 200,
        last_error: "",
        next_retry_at: null,
        delivered_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      } satisfies WebhookDelivery);
    },
  });
