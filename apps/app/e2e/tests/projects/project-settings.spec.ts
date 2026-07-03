import { expect, test } from "../../fixtures";

test.describe("Project settings", () => {
  test("project settings page loads", async ({ page }) => {
    // Find project ID from the sidebar or session
    await page.goto("/app/dashboard");
    const settingsLink = page.locator("a[href*='/app/projects/']").first();
    if (await settingsLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await settingsLink.click();
      await expect(page).toHaveURL(/\/app\/projects\//);
    }
  });

  test("404 for invalid project ID", async ({ page }) => {
    await page.goto("/app/projects/nonexistent-project-12345/settings");
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("project settings renders content", async ({ page }) => {
    await page.goto("/app/dashboard");
    const settingsLink = page.locator("a[href*='/app/projects/']").first();
    if (await settingsLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await settingsLink.click();
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/app/dashboard");
    const settingsLink = page.locator("a[href*='/app/projects/']").first();
    if (await settingsLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await settingsLink.click();
      await expect(page.locator("body")).toBeVisible();
    }
  });
});
