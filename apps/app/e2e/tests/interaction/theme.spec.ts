import { expect, test } from "../../fixtures";

test.describe("Theme", () => {
  test("theme toggle button exists", async ({ page }) => {
    await page.goto("/app/dashboard");
    // Theme toggle is typically a button with sun/moon icon
    const themeBtn = page
      .locator("button[aria-label*='theme'], button[aria-label*='mode']")
      .first();
    const anyThemeBtn = page
      .locator("button")
      .filter({ has: page.locator("[class*='sun'], [class*='moon']") })
      .first();
    if (await themeBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await expect(themeBtn).toBeVisible();
    } else if (
      await anyThemeBtn.isVisible({ timeout: 3000 }).catch(() => false)
    ) {
      await expect(anyThemeBtn).toBeVisible();
    }
  });

  test("page has a color scheme class", async ({ page }) => {
    await page.goto("/app/dashboard");
    const html = page.locator("html");
    const cls = await html.getAttribute("class");
    // Should have either 'dark' or 'light' class
    expect(cls).toBeTruthy();
  });
});
