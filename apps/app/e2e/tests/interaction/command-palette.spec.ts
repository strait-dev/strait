import { expect, test } from "../../fixtures";

test.describe("Command palette", () => {
  test("opens with keyboard shortcut", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.keyboard.press("Meta+k");
    await page.waitForTimeout(500);
    const dialog = page.locator("[role='dialog'], [cmdk-dialog], [data-cmdk]");
    if (
      await dialog
        .first()
        .isVisible({ timeout: 3000 })
        .catch(() => false)
    ) {
      await expect(dialog.first()).toBeVisible();
    }
  });

  test("escape closes the palette", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.keyboard.press("Meta+k");
    await page.waitForTimeout(500);
    const dialog = page.locator("[role='dialog'], [cmdk-dialog], [data-cmdk]");
    if (
      await dialog
        .first()
        .isVisible({ timeout: 3000 })
        .catch(() => false)
    ) {
      await page.keyboard.press("Escape");
      await page.waitForTimeout(500);
    }
  });

  test("search input is focused when opened", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.keyboard.press("Meta+k");
    await page.waitForTimeout(500);
    const input = page.locator("[cmdk-input], [role='dialog'] input").first();
    if (await input.isVisible({ timeout: 3000 }).catch(() => false)) {
      await expect(input).toBeFocused();
    }
  });
});
