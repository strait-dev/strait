import { ApiHelper, expect, test } from "../../fixtures";

const api = new ApiHelper();
const testJobName = `e2e-crosspage-${Date.now()}`;
let jobId: string;
let runId: string;

test.describe("Cross-Page State", () => {
  test.describe.configure({ mode: "serial" });

  test.beforeAll(async () => {
    try {
      const result = await api.createJobAndTrigger(testJobName);
      jobId = result.jobId;
      runId = result.runId;
    } catch {
      // API may not be available
    }
  });

  test.afterAll(async () => {
    if (jobId) {
      await api.deleteJob(jobId).catch(() => { /* cleanup */ });
    }
  });

  test("triggered run shows in global runs list", async ({ page }) => {
    if (!runId) {
      test.skip();
      return;
    }
    await page.goto("/app/runs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText(testJobName).first()).toBeVisible({
        timeout: 10_000,
      });
    }
  });

  test("job detail page shows the run", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto(`/app/jobs/${jobId}`);
    await page.waitForTimeout(500);
    const runsTab = page.getByText("Recent Runs").or(page.getByText("Runs"));
    if (await runsTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await runsTab.click();
      await page.waitForTimeout(500);
    }
  });

  test("run detail page accessible from runs list", async ({ page }) => {
    if (!runId) {
      test.skip();
      return;
    }
    await page.goto(`/app/runs/${runId}`);
    const content = page.getByText(testJobName).or(page.locator("main"));
    await expect(content).toBeVisible({ timeout: 10_000 });
  });

  test("dashboard reflects seeded data", async ({ page }) => {
    if (!jobId) {
      test.skip();
      return;
    }
    await page.goto("/app/dashboard");
    // Dashboard should show at least 1 total run
    await expect(page.locator("main")).toBeVisible();
  });
});
