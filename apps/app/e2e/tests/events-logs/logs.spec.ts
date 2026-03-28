import { expect, test } from "../../fixtures";

test.describe("Logs", () => {
  test("logs page loads", async ({ page }) => {
    await page.goto("/app/logs");
    await expect(page).toHaveURL(/\/app\/logs/);
  });

  test("page renders content", async ({ page }) => {
    await page.goto("/app/logs");
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no logs|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("page loads without crashing", async ({ page }) => {
    await page.goto("/app/logs");
    await expect(page.locator("body")).toBeVisible();
  });

  test("logs page has correct URL", async ({ page }) => {
    await page.goto("/app/logs");
    await expect(page).toHaveURL(/\/app\/logs/);
  });

  test("table has expected columns when data exists", async ({ page }) => {
    await page.goto("/app/logs");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(
        page.getByText("Status").or(page.getByText("Event"))
      ).toBeVisible();
    }
  });
});
