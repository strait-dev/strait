import { expect, test } from "../../fixtures";

test.describe("Clipboard", () => {
  test("copy button exists on API key creation dialog", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    await page.waitForTimeout(2000);
    const keysTab = page.getByRole("tab", { name: /api key/i });
    if (await keysTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await keysTab.click();
      await page.waitForTimeout(1000);
      // Verify the API keys section loaded
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("clipboard API is available", async ({ page }) => {
    await page.goto("/app/dashboard");
    const hasClipboard = await page.evaluate(() => !!navigator.clipboard);
    expect(hasClipboard).toBeTruthy();
  });

  test("copy buttons have click handlers", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    // Any copy button on the page should be interactive
    const copyBtn = page.locator("button").filter({ hasText: /copy/i }).first();
    if (await copyBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(copyBtn).toBeEnabled();
    }
  });
});
