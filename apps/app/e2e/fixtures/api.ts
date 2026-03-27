/** API helper for seeding and cleaning up test data via the Go backend. */
export class ApiHelper {
  private readonly baseUrl: string;
  private readonly secret: string;
  private projectId: string | null = null;

  constructor() {
    this.baseUrl = process.env.STRAIT_API_URL ?? "http://localhost:8080";
    this.secret = process.env.INTERNAL_SECRET ?? "";
  }

  setProjectId(id: string) {
    this.projectId = id;
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
    endpoint_url: string;
    max_attempts?: number;
    timeout_secs?: number;
  }) {
    return this.request<{ id: string; name: string }>("POST", "/v1/jobs", data);
  }

  triggerJob(id: string, payload?: unknown) {
    return this.request("POST", `/v1/jobs/${id}/trigger`, { payload });
  }

  deleteJob(id: string) {
    return this.request("DELETE", `/v1/jobs/${id}`);
  }

  // Runs
  listRuns(params?: { job_id?: string; limit?: number }) {
    const query = new URLSearchParams();
    if (params?.job_id) {
      query.set("job_id", params.job_id);
    }
    if (params?.limit) {
      query.set("limit", String(params.limit));
    }
    const qs = query.toString();
    return this.request("GET", `/v1/runs${qs ? `?${qs}` : ""}`);
  }

  cancelRun(id: string) {
    return this.request("DELETE", `/v1/runs/${id}`);
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
  createWorkflow(data: { name: string; steps?: unknown[] }) {
    return this.request<{ id: string }>("POST", "/v1/workflows", data);
  }

  deleteWorkflow(id: string) {
    return this.request("DELETE", `/v1/workflows/${id}`);
  }
}
