import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

let data: TestDataFactory;
let jobId: string;
let jobName: string;
let runId: string;

test.describe("Cross-page backend state", () => {
  test.describe.configure({ timeout: 90_000 });
  test.setTimeout(90_000);

  test.beforeAll(async () => {
    const api = new ApiHelper();
    data = new TestDataFactory(api);
    const job = await data.job("crosspage");
    const run = await api.triggerJob(job.id, { source: "cross-page-e2e" });
    jobId = job.id;
    jobName = job.name;
    runId = run.id;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("same seeded run is visible across dashboard, jobs, runs, and detail pages", async ({
    page,
  }) => {
    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
    await expect(page.getByText(runId.slice(0, 8))).toBeVisible({
      timeout: 15_000,
    });

    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: jobName })).toBeVisible();
    await selectTab(page, "Recent runs");
    await expect(
      page.getByRole("link", { name: runId.slice(0, 8) }).first()
    ).toBeVisible();

    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(runId.slice(0, 8));
    await expect(
      page.getByRole("link", { name: runId.slice(0, 8) }).first()
    ).toBeVisible();

    await gotoAndExpect(
      page,
      `/app/runs/${runId}`,
      page.getByRole("heading", { name: runId })
    );
    await expect(page.getByRole("heading", { name: runId })).toBeVisible();
  });
});
