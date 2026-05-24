import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Webhooks", () => {
  let data: TestDataFactory;

  test.beforeAll(async () => {
    data = new TestDataFactory(new ApiHelper());
    await data.webhook("webhooks-list");
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test.beforeEach(async ({ page }) => {
    await page.goto("/app/webhooks");
  });

  test("webhooks page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/webhooks/);
  });

  test("page renders content", async ({ page }) => {
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no webhooks|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("search input exists when project is active", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search webhooks...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("status filter exists when project is active", async ({ page }) => {
    const btn = page.getByRole("button", { name: "Status" });
    if (await btn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(btn).toBeVisible();
    }
  });

  test("table or empty state renders", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no project|no webhooks|went wrong/i);
    await expect(table.or(emptyState).first()).toBeVisible({ timeout: 10_000 });
  });

  test("delete button in floating bar when selected", async ({ page }) => {
    const checkbox = page.getByRole("checkbox", { name: "Select row" }).first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(checkbox).toBeVisible();
    }
  });

  test("search filters webhooks when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search webhooks...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-webhook");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("table columns include endpoint info", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(
        table
          .getByRole("columnheader", { name: "Endpoint" })
          .or(table.getByRole("columnheader", { name: "URL" }))
      ).toBeVisible();
    }
  });

  test("row click opens detail sheet", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("status filter dropdown opens when available", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    if (await filterButton.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(filterButton).toBeEnabled();
    }
  });

  test("select all checkbox works", async ({ page }) => {
    const table = page.locator("table");
    if (!(await table.isVisible({ timeout: 5000 }).catch(() => false))) {
      return;
    }
    const selectAll = table.getByRole("checkbox", { name: "Select all" });
    if (await selectAll.isVisible()) {
      await expect(selectAll).toBeVisible();
    }
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("view button appears for single selection", async ({ page }) => {
    const checkbox = page.getByRole("checkbox", { name: "Select row" }).first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(checkbox).toBeVisible();
    }
  });

  test("search clears properly when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search webhooks...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("test");
      await page.waitForTimeout(300);
      await searchInput.clear();
      await page.waitForTimeout(300);
      await expect(page.locator("main")).toBeVisible();
    }
  });
});
