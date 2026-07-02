import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let completedRunId: string;
let failedRunId: string;
let completedJobName: string;
let failedJobName: string;

async function createRunFixture({
  prefix,
  jobOverrides = {},
  payload,
  targetStatuses,
}: {
  prefix: string;
  jobOverrides?: Partial<Parameters<ApiHelper["createJob"]>[0]>;
  payload: Record<string, string>;
  targetStatuses: string[];
}) {
  let lastError: unknown;

  for (let attempt = 1; attempt <= 2; attempt += 1) {
    const job = await data.job(`${prefix}-${attempt}`, jobOverrides);
    const run = await api.triggerJob(job.id, payload);

    try {
      await api.waitForRunStatus(run.id, targetStatuses, 90_000);
      return { job, run };
    } catch (error) {
      lastError = error;
    }
  }

  throw lastError;
}

test.describe("Jobs and runs lifecycle", () => {
  test.describe.configure({ timeout: 210_000 });
  test.setTimeout(210_000);

  test.beforeAll(async ({ browserName }, testInfo) => {
    testInfo.annotations.push({ description: browserName, type: "browser" });
    testInfo.setTimeout(210_000);

    api = new ApiHelper();
    data = new TestDataFactory(api);

    const { job: completedJob, run: completedRun } = await createRunFixture({
      prefix: "completed-run",
      payload: { expected: "success" },
      targetStatuses: ["completed", "succeeded"],
    });
    const { job: failedJob, run: failedRun } = await createRunFixture({
      prefix: "failed-run",
      jobOverrides: {
        endpoint_url: api.fakeEndpoint("/status/400"),
        max_attempts: 1,
        timeout_secs: 5,
      },
      payload: { expected: "failure" },
      targetStatuses: ["failed", "dead_letter"],
    });

    completedRunId = completedRun.id;
    failedRunId = failedRun.id;
    completedJobName = completedJob.name;
    failedJobName = failedJob.name;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("lists seeded jobs and supports search", async ({ page }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

    await page.getByLabel("Search").fill(completedJobName);
    await expect(page.getByText(completedJobName)).toBeVisible();

    await page.getByLabel("Search").fill(failedJobName);
    await expect(page.getByText(failedJobName)).toBeVisible();
  });

  test("shows completed and failed runs from the real worker", async ({
    page,
  }) => {
    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Runs" })).toBeVisible();
    await expect(
      page.getByRole("link", { name: completedRunId.slice(0, 8) }).first()
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: failedRunId.slice(0, 8) }).first()
    ).toBeVisible();
    await expect(page.getByText(/completed|succeeded/i).first()).toBeVisible();
    await expect(page.getByText(/failed|dead letter/i).first()).toBeVisible();
  });

  test("opens run detail pages with job and status context", async ({
    page,
  }) => {
    await page.goto(`/app/runs/${completedRunId}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: completedRunId })
    ).toBeVisible();

    await page.goto(`/app/runs/${failedRunId}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: failedRunId })
    ).toBeVisible();
  });
});
