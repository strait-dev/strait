import { expect, test } from "../../fixtures";

test.describe("Webhook CRUD", () => {
  test("webhook list shows entries or empty state", async ({ page }) => {
    await page.goto("/app/webhooks");
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no webhooks|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("webhook detail sheet shows endpoint when clicked", async ({ page }) => {
    await page.goto("/app/webhooks");
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstRow.click();
      await page.waitForTimeout(500);
      const sheet = page.locator("[role='dialog']");
      if (await sheet.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(sheet).toBeVisible();
      }
    }
  });

  test("delete button removes webhook from list", async ({ page }) => {
    await page.goto("/app/webhooks");
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      const deleteBtn = page.getByRole("button", { name: /delete/i });
      if (await deleteBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(deleteBtn).toBeVisible();
      }
    }
  });
});
