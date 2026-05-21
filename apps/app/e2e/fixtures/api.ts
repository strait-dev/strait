import fs from "node:fs";

/** API helper for seeding and cleaning up test data via the Go backend. */
export class ApiHelper {
  private readonly baseUrl: string;
  private readonly secret: string;
  private projectId: string | null = null;
  private readonly orgId: string | null = null;

  constructor() {
    this.baseUrl = process.env.STRAIT_API_URL ?? "http://localhost:8080";
    this.secret = process.env.INTERNAL_SECRET ?? "";

    // Auto-load project ID from global-setup
    try {
      const ctx = JSON.parse(
        fs.readFileSync("playwright/.auth/project.json", "utf-8")
      );
      if (ctx.projectId) {
        this.projectId = ctx.projectId;
      }
      if (ctx.orgId) {
        this.orgId = ctx.orgId;
      }
    } catch {
      // project.json may not exist yet
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

  async request<T = unknown>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const headers: Record<string, string> = {
      "X-Internal-Secret": this.secret,
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

    if (!response.ok) {
      const text = await response.text();
      throw new Error(
        `API ${method} ${path} failed (${response.status}): ${text}`
      );
    }

    if (response.status === 204) {
      return {} as T;
    }

    return response.json() as Promise<T>;
  }

  // Jobs
  createJob(data: {
    name: string;
    slug?: string;
    endpoint_url: string;
    description?: string;
    max_attempts?: number;
    timeout_secs?: number;
    cron?: string;
    enabled?: boolean;
  }) {
    return this.request<{ id: string; name: string }>("POST", "/v1/jobs", {
      project_id: this.getProjectId(),
      slug: data.slug ?? slugFromName(data.name),
      ...data,
    });
  }

  getJob(id: string) {
    return this.request<{ id: string; name: string; enabled: boolean }>(
      "GET",
      `/v1/jobs/${id}`
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
      endpoint_url: "https://httpbin.org/post",
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
    return this.request<{ data: Array<{ id: string; status: string }> }>(
      "GET",
      `/v1/runs${qs ? `?${qs}` : ""}`
    );
  }

  cancelRun(id: string) {
    return this.request("DELETE", `/v1/runs/${id}`);
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

  // Webhooks
  createWebhook(data: { webhook_url: string; event_types: string[] }) {
    return this.request<{ id: string }>(
      "POST",
      "/v1/webhooks/subscriptions",
      data
    );
  }

  deleteWebhook(id: string) {
    return this.request("DELETE", `/v1/webhooks/subscriptions/${id}`);
  }

  // Workflows
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

  deleteWorkflow(id: string) {
    return this.request("DELETE", `/v1/workflows/${id}`);
  }

  triggerWorkflow(id: string, payload?: unknown) {
    return this.request<{ id: string; status: string }>(
      "POST",
      `/v1/workflows/${id}/trigger`,
      { project_id: this.getProjectId(), payload }
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
    return this.request("POST", `/v1/runs/dlq/${id}/replay`);
  }

  purgeDlqEntry(id: string) {
    return this.request("DELETE", `/v1/runs/dlq/${id}`);
  }

  // API Keys
  createApiKey(data: { name: string; scopes?: string[] }) {
    return this.request<{ id: string; key: string }>(
      "POST",
      "/v1/api-keys",
      data
    );
  }

  revokeApiKey(id: string) {
    return this.request("DELETE", `/v1/api-keys/${id}`);
  }
}

function slugFromName(name: string) {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return slug || `e2e-${Date.now()}`;
}
