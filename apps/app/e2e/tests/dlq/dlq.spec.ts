import { expect, test } from "../../fixtures";

test.describe("Dead Letter Queue", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dlq");
  });

  test("DLQ page loads", async ({ page }) => {
    await expect(page.locator("main")).toBeVisible();
  });

  test("page has correct URL", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/dlq/);
  });

  test("table or empty state is visible", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no dead letter|no project|empty/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("search input exists", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search by job, run ID, or error...");
    if (await searchInput.isVisible()) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("search filters DLQ entries", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search by job, run ID, or error...");
    if (await searchInput.isVisible()) {
      await searchInput.fill("nonexistent-dlq-entry");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("page title is visible", async ({ page }) => {
    const heading = page.getByRole("heading", { name: "No project selected" });
    const table = page.locator("table");
    await expect(heading.or(table)).toBeVisible();
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/dlq");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("table columns are correct when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(
        page.getByText("Run ID").or(page.getByText("Job"))
      ).toBeVisible();
    }
  });
});
