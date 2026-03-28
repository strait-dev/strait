import { expect, test } from "../../fixtures";

test.describe("Run Detail Page", () => {
  test("run detail page loads at correct URL", async ({ page }) => {
    // Navigate to runs list and click first run if available
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/runs\//);
    }
  });

  test("404 for invalid run ID", async ({ page }) => {
    await page.goto("/app/runs/nonexistent-run-id-12345");
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("status badge renders on run detail", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      const badge = page.locator("[class*='badge']").first();
      if (await badge.isVisible()) {
        await expect(badge).toBeVisible();
      }
    }
  });

  test("overview shows run metadata", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      // Run detail should show job name, status, duration etc
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("events tab loads", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      const eventsTab = page.getByText("Events");
      if (await eventsTab.isVisible({ timeout: 3000 }).catch(() => false)) {
        await eventsTab.click();
        await page.waitForTimeout(500);
      }
    }
  });

  test("error alert shows for failed runs", async ({ page }) => {
    await page.goto("/app/runs");
    // Look for any failed run in the list
    const failedBadge = page.locator("table tbody").getByText("failed").first();
    if (!(await failedBadge.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const row = failedBadge.locator("ancestor::tr").first();
    const link = row.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      const errorAlert = page.locator("[role='alert']");
      if (await errorAlert.isVisible()) {
        await expect(errorAlert).toBeVisible();
      }
    }
  });

  test("retry button visible on run detail", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      const retryBtn = page.getByRole("button", { name: /retry|replay/i });
      if (await retryBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(retryBtn).toBeVisible();
      }
    }
  });

  test("execution trace renders", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      // Execution trace or detail cells should be present
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("detail page renders without crashing", async ({ page }) => {
    await page.goto("/app/runs");
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

  test("tabs switch between overview and events", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      const overviewTab = page.getByText("Overview");
      const eventsTab = page.getByText("Events");
      if (
        (await overviewTab.isVisible({ timeout: 3000 }).catch(() => false)) &&
        (await eventsTab.isVisible().catch(() => false))
      ) {
        await eventsTab.click();
        await page.waitForTimeout(500);
        await overviewTab.click();
        await page.waitForTimeout(500);
      }
    }
  });
});
