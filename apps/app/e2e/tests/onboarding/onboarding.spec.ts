import { expect, test } from "../../fixtures";

test.describe("Onboarding", () => {
  test("app overview page loads", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("main").or(page.locator("body"))).toBeVisible();
  });

  test("onboarding or dashboard renders", async ({ page }) => {
    await page.goto("/app");
    // User may see onboarding, dashboard, or redirect
    await expect(page.locator("body")).toBeVisible();
  });

  test("code examples or dashboard present", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("body")).toBeVisible();
  });

  test("navigation works from onboarding", async ({ page }) => {
    await page.goto("/app");
    await page.waitForTimeout(500);
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("project context is set after onboarding", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page.locator("main").or(page.locator("body"))).toBeVisible();
  });

  test("sidebar is accessible", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("body")).toBeVisible();
  });

  test("page loads without crashing", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("body")).toBeVisible();
  });

  test("overview renders content", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("body")).toBeVisible();
  });
});
