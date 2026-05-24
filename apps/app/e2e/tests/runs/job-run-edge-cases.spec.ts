import { randomUUID } from "node:crypto";
import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Job run edge cases", () => {
  test.describe.configure({ timeout: 180_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("retries a transient endpoint failure and renders scheduled retry state", async ({
    page,
  }) => {
    const retryKey = randomUUID();
    const job = await data.job("retry-then-success", {
      endpoint_url: api.fakeEndpoint(
        `/retry-then-success?key=${retryKey}&failures=1`
      ),
      max_attempts: 2,
      retry_strategy: "fixed",
      retry_delays_secs: [1],
      timeout_secs: 5,
    });

    const run = await api.triggerJob(job.id, { scenario: "retry" });
    data.cleanup.add(() => api.cancelRun(run.id).catch(() => undefined));
    await expect(async () => {
      const current = await api.getRun(run.id);
      expect(current.attempt).toBeGreaterThanOrEqual(2);
      expect([
        "queued",
        "waiting",
        "delayed",
        "completed",
        "succeeded",
        "dead_letter",
      ]).toContain(current.status);
    }).toPass({ timeout: 90_000 });

    await page.goto(`/app/runs/${run.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: run.id })).toBeVisible();
    await expect(
      page
        .getByText(/queued|waiting|delayed|completed|succeeded|dead letter/i)
        .first()
    ).toBeVisible();
    await expect(page.getByText("Attempt")).toBeVisible();
  });

  test("times out slow endpoints and exposes timeout context in run detail", async ({
    page,
  }) => {
    const job = await data.job("timeout", {
      endpoint_url: api.fakeEndpoint("/timeout?delay_ms=5000"),
      max_attempts: 1,
      timeout_secs: 1,
    });

    const run = await api.triggerJob(job.id, { scenario: "timeout" });
    const failed = await api.waitForRunStatus(
      run.id,
      ["failed", "timed_out", "dead_letter"],
      90_000
    );
    expect(["failed", "timed_out", "dead_letter"]).toContain(failed.status);

    await page.goto(`/app/runs/${run.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: run.id })).toBeVisible();
    await expect(
      page.getByText(/failed|timed out|dead letter/i).first()
    ).toBeVisible();
    await expect(page.getByText("What happened")).toBeVisible();
  });

  test("cancels an active long-running job run through the Go API", async () => {
    const job = await data.job("cancel-run", {
      endpoint_url: api.fakeEndpoint("/timeout?delay_ms=10000"),
      max_attempts: 1,
      timeout_secs: 15,
    });

    const run = await api.triggerJob(job.id, { scenario: "cancel" });
    await expect(async () => {
      const current = await api.getRun(run.id);
      expect(["queued", "dequeued", "executing", "waiting"]).toContain(
        current.status
      );
    }).toPass({ timeout: 30_000 });

    await api.cancelRun(run.id);
    const canceled = await api.waitForRunStatus(run.id, ["canceled"], 60_000);
    expect(canceled.status).toBe("canceled");
  });

  test("keeps malformed trigger payloads out of the run queue", async () => {
    const job = await data.job("invalid-trigger-payload");
    const response = await api.requestRaw(
      "POST",
      `/v1/jobs/${job.id}/trigger`,
      {
        project_id: api.getProjectId(),
        concurrency_key: "x".repeat(300),
        payload: { scenario: "invalid" },
      }
    );

    expect(response.status).toBe(422);
    const runs = await api.listRuns({ job_id: job.id, limit: 20 });
    expect(runs.data.some((run) => run.job_id === job.id)).toBe(false);
  });
});
