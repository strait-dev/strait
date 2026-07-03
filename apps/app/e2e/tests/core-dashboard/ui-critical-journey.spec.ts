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

  test("reflects a newly triggered backend run on the job detail page", async ({
    page,
  }) => {
    const before = await api.listRuns({ job_id: jobId, limit: 10 });

    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: jobName })).toBeVisible();
    await expect(
      page.getByRole("main").getByRole("button", { name: "Trigger" })
    ).toBeVisible();

    const run = await api.triggerJob(jobId, { source: "job-detail-e2e" });

    const after = await waitForAdditionalRun(before.data.length, 30_000);
    expect(after.data.length).toBeGreaterThan(before.data.length);

    await page.getByRole("tab", { name: "Recent runs" }).click();
    await expect(
      page.getByRole("link", { name: run.id.slice(0, 8) }).first()
    ).toBeVisible({
      timeout: 15_000,
    });

    await api.cancelRun(run.id).catch(() => undefined);
  });

  test("reflects backend pause and resume state on the detail page", async ({
    page,
  }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: jobName })).toBeVisible();
    await expect(
      page.getByRole("main").getByRole("button", { name: "Pause" })
    ).toBeVisible();

    await api.updateJob(jobId, { enabled: false });
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect
      .poll(async () => (await api.getJob(jobId)).enabled)
      .toBe(false);
    await expect(page.getByRole("main").getByText("Paused")).toBeVisible();
    await expect(
      page.getByRole("main").getByRole("button", { name: "Resume" })
    ).toBeVisible();

    await api.updateJob(jobId, { enabled: true });
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect.poll(async () => (await api.getJob(jobId)).enabled).toBe(true);
    await expect(
      page.getByRole("main").getByRole("button", { name: "Pause" })
    ).toBeVisible();
  });
});

async function waitForAdditionalRun(previousCount: number, timeoutMs: number) {
  let latest = await api.listRuns({ job_id: jobId, limit: 10 });
  await expect(async () => {
    latest = await api.listRuns({ job_id: jobId, limit: 10 });
    expect(latest.data.length).toBeGreaterThan(previousCount);
  }).toPass({ timeout: timeoutMs });
  return latest;
}
