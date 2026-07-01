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
    // Run teardown in LIFO order so dependent resources can be unblocked before
    // their parents are deleted.
    const errors: unknown[] = [];
    for (const task of this.tasks) {
      try {
        await task();
      } catch (error) {
        errors.push(error);
      }
    }
    this.tasks.length = 0;
    if (errors.length > 0) {
      throw new AggregateError(errors, "E2E cleanup failed");
    }
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
    this.cleanup.add(async () => {
      await this.api.deleteRunsForJob(job.id).catch(() => undefined);
      await this.api.deleteJob(job.id);
    });
    return job;
  }

  async failedJobRun(prefix: string) {
    const job = await this.job(prefix, {
      endpoint_url: this.api.fakeEndpoint("/status/400"),
      max_attempts: 1,
      timeout_secs: 5,
    });
    const run = await this.api.triggerJob(job.id, { expected: "failure" });
    await this.api.waitForRunStatus(run.id, ["failed", "dead_letter"], 60_000);
    return { job, run };
  }

  async deadLetterRun(prefix: string) {
    const job = await this.job(prefix, {
      endpoint_url: this.api.fakeEndpoint("/status/400"),
      max_attempts: 1,
      timeout_secs: 5,
    });
    const run = await this.api.triggerJob(job.id, { expected: "dead-letter" });
    const failedRun = await this.api
      .waitForRunStatus(run.id, ["failed", "dead_letter"], 15_000)
      .catch(() => run);
    if (failedRun.status !== "dead_letter") {
      await this.api.forceRunDeadLetter(
        run.id,
        `${job.name} exhausted retries in e2e`
      );
    }
    await this.waitForDlqEntry(run.id);
    return { job, run: { ...run, status: "dead_letter" } };
  }

  private async waitForDlqEntry(runId: string) {
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      const entries = await this.api.listDlqEntries({ limit: 100 });
      if (entries.data.some((entry) => entry.id === runId)) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error(`Run ${runId} did not appear in the DLQ`);
  }

  async successfulJobRun(prefix: string, timeoutMs = 30_000) {
    const job = await this.job(prefix, {
      endpoint_url: this.api.fakeEndpoint("/success"),
    });
    const run = await this.api.triggerJob(job.id, { expected: "success" });
    await this.api.waitForRunStatus(
      run.id,
      ["completed", "succeeded"],
      timeoutMs
    );
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
