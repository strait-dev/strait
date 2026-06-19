import { randomUUID } from "node:crypto";
import { Client } from "pg";
import { readRunContext } from "../support/run-context";

const defaultApiUrl = "http://localhost:8080";
const internalSecretHeader = "X-Internal-Secret";
const fallbackJobEndpointUrl = "http://127.0.0.1:0/success";

type ProjectContext = {
  projectId?: string;
  orgId?: string;
  fakeEndpointUrl?: string;
};

type EventTrigger = {
  id: string;
  event_key: string;
  source_type: string;
  workflow_run_id?: string;
  status: string;
  request_payload?: unknown;
  response_payload?: unknown;
};

type WebhookDelivery = {
  id: string;
  subscription_id?: string;
  webhook_url?: string;
  status: string;
  attempts: number;
  max_attempts?: number;
  last_status_code?: number;
  last_error?: string;
  created_at?: string;
};

type WorkflowRun = {
  id: string;
  workflow_id: string;
  status: string;
  triggered_by?: string;
  workflow_version?: number;
  created_at?: string;
};

type WorkflowStepRun = {
  id: string;
  step_ref: string;
  status: string;
  job_run_id?: string;
  error?: string;
};

type Worker = {
  id: string;
  project_id: string;
  queue_name: string;
  status: string;
};

type ApiKey = {
  id: string;
  name: string;
  key_prefix?: string;
  scopes?: string[];
};

type FakeEndpointRequest = {
  id: string;
  method: string;
  path: string;
  query: Record<string, string>;
  headers: Record<string, string | string[] | undefined>;
  body: unknown;
  received_at: string;
};

type RawApiResponse<T = unknown> = {
  ok: boolean;
  status: number;
  body: T | string | null;
  text: string;
};

/** API helper for seeding and cleaning up test data via the Go backend. */
export class ApiHelper {
  private readonly baseUrl: string;
  private readonly secret: string;
  private projectId: string | null = null;
  private readonly orgId: string | null = null;
  private readonly fakeEndpointUrl: string | null = null;

  constructor() {
    this.baseUrl = process.env.STRAIT_API_URL ?? defaultApiUrl;
    this.secret = process.env.INTERNAL_SECRET ?? "";

    const ctx = readProjectContext();
    if (ctx?.projectId) {
      this.projectId = ctx.projectId;
    }
    if (ctx?.orgId) {
      this.orgId = ctx.orgId;
    }
    if (ctx?.fakeEndpointUrl) {
      this.fakeEndpointUrl = ctx.fakeEndpointUrl;
    }
  }

  setProjectId(id: string) {
    this.projectId = id;
  }

  getProjectId() {
    if (!this.projectId) {
      throw new Error(
        "No project ID found. Ensure global setup created playwright/.auth/project.json"
      );
    }
    return this.projectId;
  }

  getOrgId() {
    if (!this.orgId) {
      throw new Error(
        "No organization ID found. Ensure global setup created playwright/.auth/project.json"
      );
    }
    return this.orgId;
  }

  getFakeEndpointUrl() {
    if (!this.fakeEndpointUrl) {
      throw new Error(
        "No fake endpoint URL found. Ensure global setup created playwright/.auth/project.json"
      );
    }
    return this.fakeEndpointUrl;
  }

  fakeEndpoint(path = "/success") {
    const base = this.fakeEndpointUrl ?? process.env.E2E_FAKE_ENDPOINT_URL;
    return base ? `${base}${path}` : fallbackJobEndpointUrl;
  }

  async listFakeEndpointRequests(params?: { name?: string }) {
    const url = new URL(`${this.getFakeEndpointUrl()}/requests`);
    if (params?.name) {
      url.searchParams.set("name", params.name);
    }
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(
        `Fake endpoint request log failed (${response.status}): ${await response.text()}`
      );
    }
    return (await response.json()) as { data: FakeEndpointRequest[] };
  }

  async clearFakeEndpointRequests() {
    await fetch(`${this.getFakeEndpointUrl()}/requests`, { method: "DELETE" });
  }

  health() {
    return this.request<{ status?: string; ok?: boolean }>("GET", "/health");
  }

  listProjects() {
    return this.request<{ data?: Array<{ id: string; name: string }> }>(
      "GET",
      "/v1/projects"
    );
  }

  createProject(data: { id: string; org_id: string; name: string }) {
    return this.request<{ id: string; name: string }>(
      "POST",
      "/v1/projects",
      data
    );
  }

  deleteProject(id: string) {
    return this.request("DELETE", `/v1/projects/${id}`);
  }

  async request<T = unknown>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const response = await this.requestRaw<T>(method, path, body);

    if (!response.ok) {
      throw new Error(
        `API ${method} ${path} failed (${response.status}): ${response.text}`
      );
    }

    return response.body as T;
  }

  /** Make an API request without throwing so tests can assert error status. */
  async requestRaw<T = unknown>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<RawApiResponse<T>> {
    const headers: Record<string, string> = {
      [internalSecretHeader]: this.secret,
      "Content-Type": "application/json",
    };

    if (this.projectId) {
      headers["X-Project-Id"] = this.projectId;
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    const text = await response.text();

    if (response.status === 204) {
      return { ok: response.ok, status: response.status, body: {} as T, text };
    }

    if (!text) {
      return { ok: response.ok, status: response.status, body: null, text };
    }

    try {
      return {
        ok: response.ok,
        status: response.status,
        body: JSON.parse(text) as T,
        text,
      };
    } catch {
      return { ok: response.ok, status: response.status, body: text, text };
    }
  }

  // Jobs
  listJobs(params?: { limit?: number; search?: string }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.search) {
      query.set("search", params.search);
    }
    const qs = query.toString();
    return this.request<{
      data: Array<{ id: string; name: string; enabled: boolean }>;
    }>("GET", `/v1/jobs${qs ? `?${qs}` : ""}`);
  }

  createJob(data: {
    name: string;
    slug?: string;
    endpoint_url?: string;
    description?: string;
    max_attempts?: number;
    timeout_secs?: number;
    retry_strategy?: string;
    retry_delays_secs?: number[];
    cron?: string;
    enabled?: boolean;
    execution_mode?: "http" | "worker";
    queue_name?: string;
  }) {
    return this.request<{ id: string; name: string }>("POST", "/v1/jobs", {
      project_id: this.getProjectId(),
      slug: data.slug ?? slugFromName(data.name),
      ...data,
    });
  }

  getJob(id: string) {
    return this.request<{
      id: string;
      name: string;
      description?: string;
      endpoint_url?: string;
      cron?: string;
      max_attempts: number;
      timeout_secs: number;
      retry_strategy?: string;
      execution_mode?: string;
      queue?: string;
      enabled: boolean;
      paused: boolean;
    }>("GET", `/v1/jobs/${id}`);
  }

  /** Update job fields through the same API path the dashboard depends on. */
  updateJob(
    id: string,
    data: {
      name?: string;
      description?: string;
      endpoint_url?: string;
      cron?: string;
      max_attempts?: number;
      timeout_secs?: number;
      retry_strategy?: string;
      execution_mode?: "http" | "worker";
      queue_name?: string;
      enabled?: boolean;
    }
  ) {
    return this.request<{ id: string; name: string; enabled: boolean }>(
      "PATCH",
      `/v1/jobs/${id}`,
      data
    );
  }

  triggerJob(id: string, payload?: unknown) {
    return this.request<{ id: string; status: string }>(
      "POST",
      `/v1/jobs/${id}/trigger`,
      { project_id: this.getProjectId(), payload }
    );
  }

  deleteJob(id: string) {
    return this.request("DELETE", `/v1/jobs/${id}`);
  }

  pauseJob(id: string) {
    return this.request("POST", `/v1/jobs/${id}/pause`);
  }

  resumeJob(id: string) {
    return this.request("POST", `/v1/jobs/${id}/resume`);
  }

  /** Create a job and immediately trigger it. Returns both IDs. */
  async createJobAndTrigger(name: string) {
    const job = await this.createJob({
      name,
      endpoint_url: this.fakeEndpoint(),
    });
    const run = await this.triggerJob(job.id);
    return { jobId: job.id, runId: run.id };
  }

  // Runs
  getRun(id: string) {
    return this.request<{
      id: string;
      job_id: string;
      status: string;
      trigger: string;
      attempt?: number;
      error?: string;
    }>("GET", `/v1/runs/${id}`);
  }

  getRunEvents(runId: string) {
    return this.request<{ data: Array<{ id: string; type: string }> }>(
      "GET",
      `/v1/runs/${runId}/events`
    );
  }

  listRuns(params?: { job_id?: string; limit?: number; status?: string }) {
    const query = new URLSearchParams();
    if (params?.job_id) {
      query.set("job_id", params.job_id);
    }
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.status) {
      query.set("status", params.status);
    }
    const qs = query.toString();
    return this.request<{
      data: Array<{ id: string; job_id?: string; status: string }>;
    }>("GET", `/v1/runs${qs ? `?${qs}` : ""}`);
  }

  getStats() {
    return this.request<{
      queued: number;
      executing: number;
      delayed: number;
    }>("GET", "/v1/stats");
  }

  getAnalytics(periodHours = 24) {
    return this.request<{
      throughput: Record<string, number>;
      health_summary: Record<string, number>;
      slowest_jobs: Record<string, unknown>[];
    }>(
      "GET",
      `/v1/analytics/performance?period_hours=${encodeURIComponent(periodHours)}`
    );
  }

  cancelRun(id: string) {
    return this.request("DELETE", `/v1/runs/${id}`);
  }

  async deleteRunsForJob(jobId: string) {
    await this.withDatabase(async (client) => {
      await client.query("DELETE FROM job_runs WHERE job_id = $1", [jobId]);
    });
  }

  replayRun(runId: string) {
    return this.request<{ id: string }>("POST", `/v1/runs/${runId}/replay`);
  }

  /** Poll until a run reaches a terminal status or timeout. */
  async waitForRunStatus(
    runId: string,
    targetStatuses: string[],
    timeoutMs = 30_000
  ) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      const run = await this.getRun(runId);
      if (targetStatuses.includes(run.status)) {
        return run;
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(
      `Run ${runId} did not reach ${targetStatuses.join("/")} within ${timeoutMs}ms`
    );
  }

  // Schedules (jobs with cron)
  createSchedule(data: {
    name: string;
    endpoint_url: string;
    cron: string;
    timeout_secs?: number;
  }) {
    return this.createJob(data);
  }

  deleteSchedule(id: string) {
    return this.deleteJob(id);
  }

  // Workers
  listWorkers(params?: { limit?: number }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    const qs = query.toString();
    return this.request<{ data: Worker[] }>(
      "GET",
      `/v1/workers${qs ? `?${qs}` : ""}`
    );
  }

  // Webhooks
  async createWebhook(data: { webhook_url: string; event_types: string[] }) {
    const result = await this.request<{
      id?: string;
      subscription?: { id: string };
    }>("POST", "/v1/webhooks/subscriptions", {
      project_id: this.getProjectId(),
      ...data,
    });
    return { id: result.id ?? result.subscription?.id ?? "" };
  }

  deleteWebhook(id: string) {
    return this.request("DELETE", `/v1/webhooks/subscriptions/${id}`);
  }

  /** List webhook subscriptions while tolerating older and newer API envelopes. */
  async listWebhooks(params?: { limit?: number; search?: string }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.search) {
      query.set("search", params.search);
    }
    const qs = query.toString();
    const response = await this.request<
      | { data?: Array<{ id: string; webhook_url: string; active: boolean }> }
      | Array<{ id: string; webhook_url: string; active: boolean }>
      | null
    >("GET", `/v1/webhooks/subscriptions${qs ? `?${qs}` : ""}`);
    return {
      data: Array.isArray(response) ? response : (response?.data ?? []),
    };
  }

  listWebhookDeliveries(params?: {
    limit?: number;
    cursor?: string;
    status?: string;
    webhook_id?: string;
  }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.cursor) {
      query.set("cursor", params.cursor);
    }
    if (params?.status) {
      query.set("status", params.status);
    }
    if (params?.webhook_id) {
      query.set("webhook_id", params.webhook_id);
    }
    const qs = query.toString();
    return this.request<{ data: WebhookDelivery[] }>(
      "GET",
      `/v1/webhooks/deliveries${qs ? `?${qs}` : ""}`
    );
  }

  /** Seed one webhook delivery for dashboard assertions when local CDC is absent. */
  async seedWebhookDelivery(data: {
    subscription_id: string;
    webhook_url: string;
    status: string;
    attempts?: number;
    max_attempts?: number;
    last_status_code?: number;
    last_error?: string;
    next_retry_at?: string | null;
    event_type?: string;
  }): Promise<WebhookDelivery> {
    const id = randomUUID();
    const payload = {
      event_type: data.event_type ?? "workflow.completed",
      seeded_by: "apps-app-e2e",
      subscription_id: data.subscription_id,
    };
    await this.withDatabase(async (client) => {
      await client.query(
        `
          INSERT INTO webhook_deliveries (
            id,
            webhook_url,
            status,
            attempts,
            max_attempts,
            last_status_code,
            last_error,
            next_retry_at,
            delivered_at,
            subscription_id,
            project_id,
            payload,
            payload_size_bytes,
            event_type,
            created_at,
            updated_at
          )
          VALUES (
            $1,
            $2,
            $3,
            $4,
            $5,
            $6,
            $7,
            $8,
            NULL,
            $9,
            $10,
            $11::jsonb,
            octet_length($11::jsonb::text),
            $12,
            NOW(),
            NOW()
          )
        `,
        [
          id,
          data.webhook_url,
          data.status,
          data.attempts ?? 1,
          data.max_attempts ?? 3,
          data.last_status_code ?? null,
          data.last_error ?? null,
          data.next_retry_at ?? null,
          data.subscription_id,
          this.getProjectId(),
          JSON.stringify(payload),
          data.event_type ?? "workflow.completed",
        ]
      );
    });
    return {
      id,
      subscription_id: data.subscription_id,
      webhook_url: data.webhook_url,
      status: data.status,
      attempts: data.attempts ?? 1,
      max_attempts: data.max_attempts ?? 3,
      last_status_code: data.last_status_code,
      last_error: data.last_error,
    };
  }

  /** Seed a due pending delivery and let the real Go delivery worker process it. */
  seedPendingWebhookDelivery(data: {
    subscription_id: string;
    webhook_url: string;
    event_type?: string;
  }) {
    return this.seedWebhookDelivery({
      subscription_id: data.subscription_id,
      webhook_url: data.webhook_url,
      status: "pending",
      attempts: 0,
      max_attempts: 3,
      next_retry_at: new Date().toISOString(),
      event_type: data.event_type,
    });
  }

  /** Remove a delivery inserted by seedWebhookDelivery. */
  async deleteWebhookDelivery(id: string) {
    await this.withDatabase(async (client) => {
      await client.query("DELETE FROM webhook_deliveries WHERE id = $1", [id]);
    });
  }

  /** Run direct database setup that cannot be produced without CDC locally. */
  private async withDatabase<T>(fn: (client: Client) => Promise<T>) {
    const client = new Client({
      connectionString:
        process.env.DATABASE_URL ??
        "postgres://strait:strait@localhost:15432/strait?sslmode=disable",
    });
    await client.connect();
    try {
      return await fn(client);
    } finally {
      await client.end();
    }
  }

  /** Poll project webhook deliveries until one matches an observable condition. */
  async waitForWebhookDelivery(
    predicate: (delivery: WebhookDelivery) => boolean,
    timeoutMs = 30_000
  ) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      const deliveries = await this.listWebhookDeliveries({ limit: 100 });
      const match = deliveries.data.find(predicate);
      if (match) {
        return match;
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(
      `No webhook delivery matched predicate within ${timeoutMs}ms`
    );
  }

  // Events
  listEvents(params?: {
    limit?: number;
    cursor?: string;
    status?: string;
    workflow_run_id?: string;
    source_type?: string;
  }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.cursor) {
      query.set("cursor", params.cursor);
    }
    if (params?.status) {
      query.set("status", params.status);
    }
    if (params?.workflow_run_id) {
      query.set("workflow_run_id", params.workflow_run_id);
    }
    if (params?.source_type) {
      query.set("source_type", params.source_type);
    }
    const qs = query.toString();
    return this.request<{ data: EventTrigger[] }>(
      "GET",
      `/v1/events${qs ? `?${qs}` : ""}`
    );
  }

  getEvent(eventKey: string) {
    return this.request<EventTrigger>(
      "GET",
      `/v1/events/${encodeURIComponent(eventKey)}`
    );
  }

  sendEvent(eventKey: string, payload?: unknown) {
    return this.request<EventTrigger>(
      "POST",
      `/v1/events/${encodeURIComponent(eventKey)}/send`,
      { payload }
    );
  }

  /** Poll project event triggers until one matches an observable condition. */
  async waitForEventTrigger(
    predicate: (event: EventTrigger) => boolean,
    timeoutMs = 30_000
  ) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      const events = await this.listEvents({ limit: 100 });
      const match = events.data.find(predicate);
      if (match) {
        return match;
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(`No event trigger matched predicate within ${timeoutMs}ms`);
  }

  // Workflows
  listWorkflows(params?: { limit?: number; search?: string }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.search) {
      query.set("search", params.search);
    }
    const qs = query.toString();
    return this.request<{ data: Array<{ id: string; name: string }> }>(
      "GET",
      `/v1/workflows${qs ? `?${qs}` : ""}`
    );
  }

  createWorkflow(data: {
    name: string;
    slug?: string;
    description?: string;
    steps?: unknown[];
    enabled?: boolean;
  }) {
    return this.request<{ id: string; name: string }>("POST", "/v1/workflows", {
      project_id: this.getProjectId(),
      slug: data.slug ?? slugFromName(data.name),
      ...data,
    });
  }

  getWorkflow(id: string) {
    return this.request<{ id: string; name: string; enabled: boolean }>(
      "GET",
      `/v1/workflows/${id}`
    );
  }

  updateWorkflow(id: string, data: { enabled?: boolean }) {
    return this.request<{ id: string; name: string; enabled: boolean }>(
      "PATCH",
      `/v1/workflows/${id}`,
      data
    );
  }

  deleteWorkflow(id: string) {
    return this.request("DELETE", `/v1/workflows/${id}`);
  }

  triggerWorkflow(id: string, payload?: unknown) {
    return this.request<WorkflowRun>("POST", `/v1/workflows/${id}/trigger`, {
      project_id: this.getProjectId(),
      payload,
    });
  }

  listWorkflowRuns(
    workflowId: string,
    params?: { limit?: number; cursor?: string }
  ) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.cursor) {
      query.set("cursor", params.cursor);
    }
    const qs = query.toString();
    return this.request<{ data: WorkflowRun[] }>(
      "GET",
      `/v1/workflows/${workflowId}/runs${qs ? `?${qs}` : ""}`
    );
  }

  getWorkflowRun(id: string) {
    return this.request<WorkflowRun>("GET", `/v1/workflow-runs/${id}`);
  }

  cancelWorkflowRun(id: string) {
    return this.request("DELETE", `/v1/workflow-runs/${id}`);
  }

  listWorkflowStepRuns(workflowRunId: string, params?: { limit?: number }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    const qs = query.toString();
    return this.request<{ data: WorkflowStepRun[] }>(
      "GET",
      `/v1/workflow-runs/${workflowRunId}/steps${qs ? `?${qs}` : ""}`
    );
  }

  /** Poll until a workflow run reaches one of the expected statuses. */
  async waitForWorkflowRunStatus(
    workflowRunId: string,
    targetStatuses: string[],
    timeoutMs = 30_000
  ) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      const run = await this.getWorkflowRun(workflowRunId);
      if (targetStatuses.includes(run.status)) {
        return run;
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(
      `Workflow run ${workflowRunId} did not reach ${targetStatuses.join(
        "/"
      )} within ${timeoutMs}ms`
    );
  }

  // DLQ
  listDlqEntries(params?: { limit?: number; search?: string }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    if (params?.search) {
      query.set("search", params.search);
    }
    const qs = query.toString();
    return this.request<{ data: Array<{ id: string }> }>(
      "GET",
      `/v1/runs/dlq${qs ? `?${qs}` : ""}`
    );
  }

  replayDlqEntry(id: string) {
    return this.request("POST", `/v1/runs/${id}/dlq-replay`);
  }

  purgeDlqEntry(id: string) {
    return this.request("POST", `/v1/admin/dlq/${id}/purge`);
  }

  /** Force a run into DLQ for deterministic dashboard coverage. */
  async forceRunDeadLetter(runId: string, error = "e2e forced dead letter") {
    await this.withDatabase(async (client) => {
      await client.query(
        `
          UPDATE job_runs
          SET status = 'dead_letter',
              error = $2,
              error_class = 'client',
              attempt = GREATEST(attempt, 1),
              finished_at = NOW()
          WHERE id = $1
        `,
        [runId, error]
      );
    });
  }

  // API Keys
  listApiKeys(params?: { limit?: number }) {
    const query = new URLSearchParams();
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    const qs = query.toString();
    return this.request<{ data: ApiKey[] }>(
      "GET",
      `/v1/api-keys${qs ? `?${qs}` : ""}`
    );
  }

  createApiKey(data: {
    expires_in_days?: number;
    name: string;
    scopes?: string[];
  }) {
    return this.request<{ id: string; name: string; key: string }>(
      "POST",
      "/v1/api-keys",
      {
        project_id: this.getProjectId(),
        ...data,
      }
    );
  }

  revokeApiKey(id: string) {
    return this.request("DELETE", `/v1/api-keys/${id}`);
  }
}

/** Read the project and organization created by global setup, if available. */
function readProjectContext(): ProjectContext | null {
  return readRunContext();
}

function slugFromName(name: string) {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return slug || `e2e-${Date.now()}`;
}
