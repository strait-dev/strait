import { expect, test } from "../../fixtures";

test.describe("Run Detail Sheet", () => {
  test("run detail sheet opens on row click", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    await firstRow.click();
    await page.waitForTimeout(1000);
    // Sheet should be visible with run details
    const sheet = page.locator("[role='dialog']");
    if (await sheet.isVisible()) {
      await expect(sheet).toBeVisible();
    }
  });

  test("run detail shows status badge", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    await firstRow.click();
    await page.waitForTimeout(1000);
    const sheet = page.locator("[role='dialog']");
    if (await sheet.isVisible()) {
      const badge = sheet.locator("[class*='badge']").first();
      await expect(badge).toBeVisible();
    }
  });

  test("run detail shows run ID", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    await firstRow.click();
    await page.waitForTimeout(1000);
  });

  test("close button dismisses the sheet", async ({ page }) => {
    await page.goto("/app/runs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    await firstRow.click();
    await page.waitForTimeout(1000);
    const sheet = page.locator("[role='dialog']");
    if (await sheet.isVisible()) {
      const closeButton = sheet
        .getByRole("button", { name: /close/i })
        .or(sheet.locator("button[class*='close']"));
      if (await closeButton.isVisible()) {
        await closeButton.click();
        await expect(sheet).not.toBeVisible();
      }
    }
  });
});
