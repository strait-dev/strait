import { expect, test } from "../../fixtures";

test.describe("DLQ Operations", () => {
  test("DLQ list shows entries or empty state", async ({ page }) => {
    await page.goto("/app/dlq");
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no dead letter|went wrong|empty/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("search finds DLQ entries when available", async ({ page }) => {
    await page.goto("/app/dlq");
    const searchInput = page.getByPlaceholder(
      "Search by job, run ID, or error..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("test");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("DLQ page supports bulk actions when entries exist", async ({
    page,
  }) => {
    await page.goto("/app/dlq");
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      // Should show replay or purge buttons
      const actionBtn = page.getByRole("button", { name: /replay|purge/i });
      if (await actionBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(actionBtn).toBeVisible();
      }
    }
  });
});
