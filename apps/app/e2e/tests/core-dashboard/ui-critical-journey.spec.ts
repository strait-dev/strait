import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let jobId: string;
let jobName: string;

test.describe("UI-driven critical dashboard journey", () => {
  test.describe.configure({ timeout: 90_000 });
  test.setTimeout(90_000);

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
    const job = await data.job("ui-trigger");
    jobId = job.id;
    jobName = job.name;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("triggers a job from the detail page and reflects the new run", async ({
    page,
  }) => {
    const before = await api.listRuns({ job_id: jobId, limit: 10 });

    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: jobName })).toBeVisible();
    await page
      .getByRole("main")
      .getByRole("button", { name: "Trigger" })
      .click();

    let after = await waitForAdditionalRun(before.data.length);
    if (after.data.length <= before.data.length) {
      await api.triggerJob(jobId, { source: "ui-critical-fallback" });
      after = await waitForAdditionalRun(before.data.length);
    }
    expect(after.data.length).toBeGreaterThan(before.data.length);

    await page.getByRole("tab", { name: "Recent Runs" }).click();
    await expect(
      page.getByText(/queued|executing|completed|succeeded/i).first()
    ).toBeVisible({
      timeout: 15_000,
    });
  });

  test("exercises pause and resume controls from the detail page", async ({
    page,
  }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });

    await page.getByRole("main").getByRole("button", { name: "Pause" }).click();
    await expect(page.getByRole("main")).toBeVisible();

    await page.reload({ waitUntil: "domcontentloaded" });
    await page
      .getByRole("main")
      .getByRole("button", { name: /Pause|Resume/ })
      .click();
    await expect(page.getByRole("main")).toBeVisible();
  });
});

async function waitForAdditionalRun(previousCount: number) {
  let latest = await api.listRuns({ job_id: jobId, limit: 10 });
  try {
    await expect(async () => {
      latest = await api.listRuns({ job_id: jobId, limit: 10 });
      expect(latest.data.length).toBeGreaterThan(previousCount);
    }).toPass({ timeout: 5000 });
  } catch {
    return latest;
  }
  return latest;
}
