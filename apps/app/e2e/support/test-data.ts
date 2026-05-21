import crypto from "node:crypto";
import type { ApiHelper } from "../fixtures/api";

type CleanupTask = () => Promise<unknown>;

/** Per-test cleanup stack for backend resources created by e2e scenarios. */
export class CleanupRegistry {
  private readonly tasks: CleanupTask[] = [];

  add(task: CleanupTask) {
    this.tasks.unshift(task);
  }

  async run() {
    await Promise.allSettled(this.tasks.map((task) => task()));
    this.tasks.length = 0;
  }
}

/** Backend data factory that creates deterministic, isolated e2e resources. */
export class TestDataFactory {
  readonly cleanup = new CleanupRegistry();
  private readonly api: ApiHelper;

  constructor(api: ApiHelper) {
    this.api = api;
  }

  name(prefix: string) {
    return `e2e-${prefix}-${crypto.randomUUID().slice(0, 8)}`;
  }

  async job(
    prefix: string,
    overrides: Partial<Parameters<ApiHelper["createJob"]>[0]> = {}
  ) {
    const job = await this.api.createJob({
      name: this.name(prefix),
      endpoint_url: this.api.fakeEndpoint("/success"),
      max_attempts: 1,
      timeout_secs: 10,
      ...overrides,
    });
    this.cleanup.add(() => this.api.deleteJob(job.id));
    return job;
  }

  async failedJobRun(prefix: string) {
    const job = await this.job(prefix, {
      endpoint_url: this.api.fakeEndpoint("/fail"),
      max_attempts: 1,
      timeout_secs: 5,
    });
    const run = await this.api.triggerJob(job.id, { expected: "failure" });
    await this.api.waitForRunStatus(run.id, ["failed"], 30_000);
    return { job, run };
  }

  async successfulJobRun(prefix: string) {
    const job = await this.job(prefix, {
      endpoint_url: this.api.fakeEndpoint("/success"),
    });
    const run = await this.api.triggerJob(job.id, { expected: "success" });
    await this.api.waitForRunStatus(run.id, ["completed", "succeeded"], 30_000);
    return { job, run };
  }

  async workflow(prefix: string, jobIds: string[]) {
    const workflow = await this.api.createWorkflow({
      name: this.name(prefix),
      steps: jobIds.map((jobId, index) => ({
        job_id: jobId,
        step_ref: `step-${index + 1}`,
        depends_on: index === 0 ? [] : [`step-${index}`],
      })),
    });
    this.cleanup.add(() => this.api.deleteWorkflow(workflow.id));
    return workflow;
  }

  async webhook(prefix: string, eventTypes = ["run.completed"]) {
    const webhook = await this.api.createWebhook({
      webhook_url: this.api.fakeEndpoint(`/echo?name=${prefix}`),
      event_types: eventTypes,
    });
    this.cleanup.add(() => this.api.deleteWebhook(webhook.id));
    return webhook;
  }
}
