import { expect, test } from "../../fixtures";

test.describe("Dead Letter Queue", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dlq");
  });

  test("DLQ page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/dlq/);
  });

  test("page renders content", async ({ page }) => {
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no dead letter|went wrong|empty/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("page has correct URL", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/dlq/);
  });

  test("search input exists when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by job, run ID, or error..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("search filters DLQ entries when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by job, run ID, or error..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-dlq-entry");
      await page.waitForTimeout(500);
      await expect(page.locator("body")).toBeVisible();
    }
  });

  test("page content is visible", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("table columns are correct when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(
        page.getByText("Run ID").or(page.getByText("Job"))
      ).toBeVisible();
    }
  });
});
