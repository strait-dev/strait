import { createServerFn } from "@tanstack/react-start";
import type {
  APIKey,
  EventTrigger,
  Job,
  JobRun,
  ListParams,
  PaginatedResponse,
  ProjectSettings,
  Region,
  RunEvent,
  WebhookDelivery,
  WebhookSubscription,
  Workflow,
  WorkflowRun,
  WorkflowStep,
} from "@/hooks/api/types";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

export const fetchJobs = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { status?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<Job>>("/v1/jobs", {
      params: {
        limit: data.limit,
        cursor: data.cursor,
        status: data.status,
        search: data.search,
      },
    });
  });

export const fetchJob = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<Job>(`/v1/jobs/${data.id}`);
  });

export const triggerJobFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { id: string; payload?: unknown; priority?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<{ id: string }>(`/v1/jobs/${data.id}/trigger`, {
      method: "POST",
      body: { payload: data.payload, priority: data.priority },
    });
  });

export const updateJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    const { id, ...body } = data;
    return apiRequest<Job>(`/v1/jobs/${id}`, { method: "PATCH", body });
  });

export const deleteJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<void>(`/v1/jobs/${data.id}`, { method: "DELETE" });
  });

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

export const fetchRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (
      data: ListParams & {
        status?: string;
        job_id?: string;
        search?: string;
      }
    ) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<JobRun>>("/v1/runs", {
      params: {
        limit: data.limit,
        cursor: data.cursor,
        status: data.status,
        job_id: data.job_id,
        search: data.search,
      },
    });
  });

export const fetchRun = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<JobRun>(`/v1/runs/${data.id}`);
  });

export const fetchRunEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { runId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<RunEvent>>(
      `/v1/runs/${data.runId}/events`,
      { params: { limit: data.limit, cursor: data.cursor } }
    );
  });

export const replayRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<{ id: string }>(`/v1/runs/${data.runId}/replay`, {
      method: "POST",
    });
  });

export const cancelRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<void>(`/v1/runs/${data.runId}`, { method: "DELETE" });
  });

// ---------------------------------------------------------------------------
// DLQ (Dead Letter Queue)
// ---------------------------------------------------------------------------

export const fetchDlqRuns = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams & { search?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<JobRun>>("/v1/runs/dlq", {
      params: { limit: data.limit, cursor: data.cursor, search: data.search },
    });
  });

export const replayDlqRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<{ id: string }>(`/v1/runs/${data.runId}/dlq-replay`, {
      method: "POST",
    });
  });

export const bulkReplayDlqFn = createServerFn({ method: "POST" })
  .inputValidator((data: { run_ids: string[] }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<{ replayed: number }>("/v1/runs/bulk-dlq-replay", {
      method: "POST",
      body: { run_ids: data.run_ids },
    });
  });

// ---------------------------------------------------------------------------
// Workflows
// ---------------------------------------------------------------------------

export const fetchWorkflows = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams & { search?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<Workflow>>("/v1/workflows", {
      params: { limit: data.limit, cursor: data.cursor, search: data.search },
    });
  });

export const fetchWorkflow = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<Workflow>(`/v1/workflows/${data.id}`);
  });

export const fetchWorkflowSteps = createServerFn({ method: "GET" })
  .inputValidator((data: { workflowId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    // Steps are returned as part of the workflow version
    const resp = await apiRequest<PaginatedResponse<WorkflowStep>>(
      `/v1/workflows/${data.workflowId}/versions`,
      { params: { limit: 1 } }
    );
    // Get the latest version's steps
    if (resp.data.length > 0) {
      const latestVersion = resp.data[0] as unknown as { id: string };
      const stepsResp = await apiRequest<PaginatedResponse<WorkflowStep>>(
        `/v1/workflows/${data.workflowId}/versions/${latestVersion.id}/steps`
      );
      return stepsResp.data;
    }
    return [] as WorkflowStep[];
  });

export const fetchWorkflowRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { workflowId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<WorkflowRun>>(
      `/v1/workflows/${data.workflowId}/runs`,
      { params: { limit: data.limit, cursor: data.cursor } }
    );
  });

export const triggerWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      workflowId: string;
      payload?: unknown;
      tags?: Record<string, string>;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<WorkflowRun>(`/v1/workflows/${data.workflowId}/trigger`, {
      method: "POST",
      body: { payload: data.payload, tags: data.tags },
    });
  });

export const updateWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    const { id, ...body } = data;
    return apiRequest<Workflow>(`/v1/workflows/${id}`, {
      method: "PATCH",
      body,
    });
  });

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

export const fetchWebhookSubscriptions = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<WebhookSubscription>>(
      "/v1/webhooks/subscriptions",
      { params: { limit: data.limit, cursor: data.cursor } }
    );
  });

export const fetchWebhookDeliveries = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<WebhookDelivery>>(
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
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<WebhookSubscription>("/v1/webhooks/subscriptions", {
      method: "POST",
      body: data,
    });
  });

export const deleteWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<void>(`/v1/webhooks/subscriptions/${data.id}`, {
      method: "DELETE",
    });
  });

export const testWebhookFn = createServerFn({ method: "POST" })
  .inputValidator((data: { webhook_url: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<WebhookDelivery>("/v1/webhooks/test", {
      method: "POST",
      body: data,
    });
  });

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

export const fetchEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { type?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<EventTrigger>>("/v1/events", {
      params: {
        limit: data.limit,
        cursor: data.cursor,
        type: data.type,
        search: data.search,
      },
    });
  });

export const fetchEvent = createServerFn({ method: "GET" })
  .inputValidator((data: { eventKey: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<EventTrigger>(`/v1/events/${data.eventKey}`);
  });

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

export const fetchApiKeys = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<APIKey>>("/v1/api-keys", {
      params: { limit: data.limit, cursor: data.cursor },
    });
  });

export const createApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { name: string; scopes: string[]; expiresInDays?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    const expiresIn = data.expiresInDays
      ? `${data.expiresInDays * 24}h`
      : undefined;
    return apiRequest<APIKey & { key: string }>("/v1/api-keys", {
      method: "POST",
      body: { name: data.name, scopes: data.scopes, expires_in: expiresIn },
    });
  });

export const revokeApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<void>(`/v1/api-keys/${data.keyId}`, {
      method: "DELETE",
    });
  });

export const rotateApiKeyFn = createServerFn({ method: "POST" })
  .inputValidator((data: { keyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<APIKey & { key: string }>(
      `/v1/api-keys/${data.keyId}/rotate`,
      { method: "POST" }
    );
  });

// ---------------------------------------------------------------------------
// Regions & Project Settings
// ---------------------------------------------------------------------------

export const fetchRegions = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PaginatedResponse<Region>>("/v1/regions");
  });

export const fetchProjectSettings = createServerFn({ method: "GET" })
  .inputValidator((data: { projectId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<ProjectSettings>(
      `/v1/projects/${data.projectId}/settings`
    );
  });

export const updateProjectSettingsFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string; default_region: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<ProjectSettings>(
      `/v1/projects/${data.projectId}/settings`,
      { method: "PUT", body: { default_region: data.default_region } }
    );
  });

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

export const fetchStats = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<Record<string, string | number | boolean | null>>("/v1/stats");
  });
