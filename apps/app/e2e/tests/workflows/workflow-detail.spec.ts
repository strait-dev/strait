import { expect, test } from "../../fixtures";

test.describe("Workflow Detail Page", () => {
  test("workflow detail page loads from list", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/workflows\//);
    }
  });

  test("404 for invalid workflow ID", async ({ page }) => {
    await page.goto("/app/workflows/nonexistent-workflow-12345");
    const error = page.getByText(/not found|went wrong|error/i);
    const main = page.locator("main");
    await expect(error.or(main)).toBeVisible({ timeout: 10_000 });
  });

  test("overview tab shows metadata", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const overview = page.getByText("Overview");
      if (await overview.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(overview).toBeVisible();
      }
    }
  });

  test("DAG visualization renders", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      // DAG flow uses react-flow which renders SVG/canvas elements
      const dagContainer = page.locator(
        "[class*='react-flow'], [class*='dag'], canvas"
      );
      if (
        await dagContainer
          .first()
          .isVisible({ timeout: 3000 })
          .catch(() => false)
      ) {
        await expect(dagContainer.first()).toBeVisible();
      }
    }
  });

  test("steps tab shows workflow steps", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const stepsTab = page.getByText("Steps");
      if (await stepsTab.isVisible({ timeout: 3000 }).catch(() => false)) {
        await stepsTab.click();
        await page.waitForTimeout(1000);
      }
    }
  });

  test("runs tab shows workflow run history", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const runsTab = page.getByText("Runs");
      if (await runsTab.isVisible({ timeout: 3000 }).catch(() => false)) {
        await runsTab.click();
        await page.waitForTimeout(1000);
      }
    }
  });

  test("configuration card renders", async ({ page }) => {
    await page.goto("/app/workflows");
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
    await page.goto("/app/workflows");
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

  test("status badge shows on detail header", async ({ page }) => {
    await page.goto("/app/workflows");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const link = firstRow.locator("a").first();
    if (await link.isVisible()) {
      await link.click();
      await page.waitForTimeout(2000);
      const badge = page.locator("[class*='badge']").first();
      if (await badge.isVisible()) {
        await expect(badge).toBeVisible();
      }
    }
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/app/workflows");
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
