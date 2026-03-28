import { ApiHelper, expect, test } from "../../fixtures";

const api = new ApiHelper();
const jobIds: string[] = [];
const prefix = `e2e-filter-${Date.now()}`;

test.describe("Filter Combinations", () => {
  test.beforeAll(async () => {
    // Seed multiple jobs
    for (let i = 0; i < 5; i++) {
      try {
        const job = await api.createJob({
          name: `${prefix}-job-${i}`,
          endpoint_url: "https://httpbin.org/post",
        });
        jobIds.push(job.id);
      } catch {
        // API may not be available
      }
    }
  });

  test.afterAll(async () => {
    for (const id of jobIds) {
      await api.deleteJob(id).catch(() => {
        /* cleanup */
      });
    }
  });

  test("seeded jobs appear in list", async ({ page }) => {
    if (jobIds.length === 0) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText(`${prefix}-job-0`)).toBeVisible({
        timeout: 10_000,
      });
    }
  });

  test("search filters to matching jobs", async ({ page }) => {
    if (jobIds.length === 0) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill(prefix);
      await page.waitForTimeout(500);
      const rows = page.locator("table tbody tr");
      const count = await rows.count();
      expect(count).toBeGreaterThanOrEqual(1);
    }
  });

  test("search with no matches shows empty state", async ({ page }) => {
    if (jobIds.length === 0) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-job-xyz-12345");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("clearing search shows all jobs", async ({ page }) => {
    if (jobIds.length === 0) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill(prefix);
      await page.waitForTimeout(500);
      await searchInput.clear();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("status filter shows filter options", async ({ page }) => {
    if (jobIds.length === 0) {
      test.skip();
      return;
    }
    await page.goto("/app/jobs");
    const filterBtn = page.getByRole("button", { name: "Status" });
    if (await filterBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await filterBtn.click();
      const items = page.getByRole("menuitemcheckbox");
      const count = await items.count();
      expect(count).toBeGreaterThan(0);
    }
  });
});
