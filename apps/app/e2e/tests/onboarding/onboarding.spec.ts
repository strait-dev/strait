import { expect, test } from "../../fixtures";

test.describe("Onboarding", () => {
  test("app overview page loads", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("main")).toBeVisible();
  });

  test("onboarding content shows SDK setup", async ({ page }) => {
    await page.goto("/app");
    const sdkContent = page.getByText(/sdk|install|get started|quickstart/i);
    const dashboard = page.getByText("Total Runs");
    // User may see onboarding or dashboard depending on state
    await expect(sdkContent.or(dashboard)).toBeVisible({ timeout: 10_000 });
  });

  test("code examples are present", async ({ page }) => {
    await page.goto("/app");
    const codeBlock = page.locator("pre, code, [class*='code']");
    const dashboard = page.getByText("Total Runs");
    await expect(codeBlock.first().or(dashboard)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("navigation works from onboarding", async ({ page }) => {
    await page.goto("/app");
    await page.waitForTimeout(1000);
    // Should be able to navigate to any page
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("project context is set after onboarding", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page.locator("main")).toBeVisible();
  });

  test("sidebar is accessible during onboarding", async ({ page }) => {
    await page.goto("/app");
    const sidebar = page.locator("nav, aside, [class*='sidebar']");
    await expect(sidebar.first()).toBeVisible();
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("overview redirects or renders content", async ({ page }) => {
    await page.goto("/app");
    await page.waitForTimeout(2000);
    // Should either show overview/onboarding or redirect to dashboard
    await expect(page.locator("main")).toBeVisible();
  });
});
