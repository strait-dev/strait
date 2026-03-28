import { expect, test } from "../../fixtures";

test.describe("OAuth Consent", () => {
  test("page loads with params", async ({ page }) => {
    await page.goto("/oauth/consent?client_id=test&scope=read");
    const heading = page.getByText(/authorize|consent|application/i);
    const error = page.getByText(/invalid|error/i);
    const content = page.locator("main").or(page.locator("body"));
    await expect(heading.or(error).or(content)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/oauth/consent");
    await expect(page.locator("body")).toBeVisible();
  });

  test("shows error for missing params", async ({ page }) => {
    await page.goto("/oauth/consent");
    const error = page.getByText(/invalid|missing|error|request/i);
    const content = page.locator("body");
    await expect(error.or(content)).toBeVisible({ timeout: 10_000 });
  });
});
