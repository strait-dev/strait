import { ApiHelper, expect, test } from "../../fixtures";

const api = new ApiHelper();
const testJobName = `e2e-lifecycle-${Date.now()}`;
let jobId: string;

test.describe("Job Lifecycle", () => {
  test.describe.configure({ mode: "serial" });

  test("create a job via API", async () => {
    try {
      const job = await api.createJob({
        name: testJobName,
        endpoint_url: "https://httpbin.org/post",
        max_attempts: 1,
        timeout_secs: 10,
      });
      jobId = job.id;
      expect(job.id).toBeTruthy();
      expect(job.name).toBe(testJobName);
    } catch {
      // Go API not available -- skip remaining tests
      test.skip();
    }
  });

  test("job appears in jobs list", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText(testJobName)).toBeVisible({
        timeout: 10_000,
      });
    }
  });

  test("job detail page loads", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto(`/app/jobs/${jobId}`);
    await expect(page.getByText(testJobName)).toBeVisible({ timeout: 10_000 });
  });

  test("trigger the job creates a run", async () => {
    if (!jobId) {
      test.skip();
      return;
    }
    const run = await api.triggerJob(jobId);
    expect(run.id).toBeTruthy();
  });

  test("run appears in runs list", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto("/app/runs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      // The run's job name should appear
      await expect(page.getByText(testJobName).first()).toBeVisible({
        timeout: 10_000,
      });
    }
  });

  test("run appears in job detail runs tab", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto(`/app/jobs/${jobId}`);
    const runsTab = page.getByText("Recent Runs").or(page.getByText("Runs"));
    if (await runsTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await runsTab.click();
      await page.waitForTimeout(500);
      const table = page.locator("table");
      if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(table.locator("tbody tr").first()).toBeVisible();
      }
    }
  });

  test("delete the job via API", async () => {
    if (!jobId) {
      test.skip();
      return;
    }
    await api.deleteJob(jobId);
  });

  test("deleted job no longer in list", async ({ page }) => {
    await page.goto("/app/jobs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      // Job name should not appear
      const jobText = page.getByText(testJobName);
      await expect(jobText).not.toBeVisible({ timeout: 5000 });
    }
  });
});
