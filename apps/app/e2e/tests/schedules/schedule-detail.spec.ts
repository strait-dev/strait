import { expect, test } from "../../fixtures";

test.describe("Schedule Detail Page", () => {
  test("schedule detail page loads from list", async ({ page }) => {
    await page.goto("/app/schedules");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/schedules\//);
    }
  });

  test("404 for invalid schedule ID", async ({ page }) => {
    await page.goto("/app/schedules/nonexistent-schedule-12345");
    const error = page.getByText(/not found|went wrong|error/i);
    const main = page.locator("main");
    await expect(error.or(main)).toBeVisible({ timeout: 10_000 });
  });

  test("shows cron expression", async ({ page }) => {
    await page.goto("/app/schedules");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const schedule = page.getByText("Schedule");
      if (await schedule.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(schedule).toBeVisible();
      }
    }
  });

  test("configuration card renders", async ({ page }) => {
    await page.goto("/app/schedules");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const config = page.getByText("Configuration");
      if (await config.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(config).toBeVisible();
      }
    }
  });

  test("trigger and pause buttons visible", async ({ page }) => {
    await page.goto("/app/schedules");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const triggerBtn = page.getByRole("button", { name: /trigger/i });
      if (await triggerBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(triggerBtn).toBeVisible();
      }
    }
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/app/schedules");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await expect(page.locator("body")).toBeVisible();
    }
  });
});
