import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let completedRunId: string;
let failedRunId: string;
let completedJobName: string;
let failedJobName: string;

test.describe("Jobs and runs lifecycle", () => {
  test.describe.configure({ timeout: 120_000 });
  test.setTimeout(120_000);

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    const completedJob = await data.job("completed-run");
    const failedJob = await data.job("failed-run", {
      endpoint_url: api.fakeEndpoint("/fail"),
      max_attempts: 1,
      timeout_secs: 5,
    });
    const completedRun = await api.triggerJob(completedJob.id, {
      expected: "success",
    });
    const failedRun = await api.triggerJob(failedJob.id, {
      expected: "failure",
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

    await page.getByPlaceholder("Search jobs...").fill(completedJobName);
    await expect(page.getByText(completedJobName)).toBeVisible();

    await page.getByPlaceholder("Search jobs...").fill(failedJobName);
    await expect(page.getByText(failedJobName)).toBeVisible();
  });

  test("shows completed and failed runs from the real worker", async ({
    page,
  }) => {
    await api.waitForRunStatus(
      completedRunId,
      ["completed", "succeeded"],
      60_000
    );

    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("table", { name: "Runs" })).toBeVisible();
    await expect(
      page.getByRole("link", { name: completedRunId.slice(0, 8) }).first()
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: failedRunId.slice(0, 8) }).first()
    ).toBeVisible();
    await expect(page.getByText(/completed|succeeded/i).first()).toBeVisible();
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
