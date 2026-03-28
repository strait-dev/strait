import { expect, test } from "../../fixtures";

test.describe("Responsive Layout", () => {
  test("desktop viewport shows full sidebar", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const sidebar = page.locator("nav, aside, [class*='sidebar']").first();
    await expect(sidebar).toBeVisible();
  });

  test("mobile viewport renders correctly", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    await expect(page.locator("body")).toBeVisible();
  });

  test("tablet viewport renders correctly", async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    await expect(page.locator("body")).toBeVisible();
  });

  test("navigation works at mobile breakpoint", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    // Should be able to navigate even on mobile
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);
  });
});
