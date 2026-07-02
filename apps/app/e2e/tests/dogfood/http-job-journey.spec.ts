import type { ApiHelper } from "../../fixtures";
import { expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood HTTP job journey", () => {
  test.describe.configure({ timeout: 120_000 });

  test("operates an HTTP job through the browser and verifies backend state", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const job = await data.job("dogfood-http-job", {
      endpoint_url: api.fakeEndpoint("/success"),
      max_attempts: 1,
      timeout_secs: 10,
      description: "Dogfood HTTP job operated through the browser",
    });

    try {
      await page.goto(`/app/jobs/${job.id}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("heading", { name: job.name })).toBeVisible();
      await expect(page.getByRole("button", { name: "Trigger" })).toBeVisible();

      const runId = await triggerJobThroughUI(page, api, job.id);

      await api.waitForRunStatus(runId, ["completed"], 60_000);

      await page.getByRole("tab", { name: "Recent Runs" }).click();
      await expect(
        page.getByRole("link", { name: runId.slice(0, 8) }).first()
      ).toBeVisible({ timeout: 15_000 });

      await setJobPausedThroughUI(page, api, job.id, true);
      await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();

      await setJobPausedThroughUI(page, api, job.id, false);
      await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();

      await page.goto(`/app/runs/${runId}`, { waitUntil: "domcontentloaded" });
      await expect(page).toHaveURL(new RegExp(`/app/runs/${runId}`));
      await expect(page.getByRole("heading", { name: runId })).toBeVisible();
      await expect(page.getByText("completed").first()).toBeVisible();
    } finally {
      await data.cleanup.run();
    }
  });
});

async function triggerJobThroughUI(
  page: import("@playwright/test").Page,
  api: ApiHelper,
  jobId: string
) {
  const button = page.getByRole("button", { name: "Trigger" });
  await expect(button).toBeVisible({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt++) {
    await button.click();
    const runId = await waitForLatestJobRunId(api, jobId, 8000).catch(
      () => undefined
    );
    if (runId) {
      return runId;
    }
  }
  return await waitForLatestJobRunId(api, jobId, 20_000);
}

async function waitForLatestJobRunId(
  api: ApiHelper,
  jobId: string,
  timeout = 20_000
) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const runs = await api.listRuns({ job_id: jobId, limit: 10 });
    const runId = runs.data[0]?.id;
    if (runId) {
      return runId;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`No run appeared for job ${jobId}`);
}

async function setJobPausedThroughUI(
  page: import("@playwright/test").Page,
  api: ApiHelper,
  jobId: string,
  paused: boolean
) {
  const buttonName = paused ? "Pause" : "Resume";
  const button = page.getByRole("button", { name: buttonName });
  await expect(button).toBeVisible({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt++) {
    await button.click();
    const matched = await expect
      .poll(async () => (await api.getJob(jobId)).paused, { timeout: 8000 })
      .toBe(paused)
      .then(
        () => true,
        () => false
      );
    if (matched) {
      return;
    }
  }
  await expect
    .poll(async () => (await api.getJob(jobId)).paused, { timeout: 15_000 })
    .toBe(paused);
}
