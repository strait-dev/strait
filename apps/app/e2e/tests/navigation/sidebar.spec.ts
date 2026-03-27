import { test, expect } from "../../fixtures";

test.describe("Sidebar Navigation", () => {
  test("dashboard link navigates correctly", async ({ page }) => {
    await page.goto("/app/jobs");
    await page.getByRole("link", { name: /dashboard/i }).click();
    await expect(page).toHaveURL(/\/app\/dashboard/);
  });

  test("jobs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: /^jobs$/i }).click();
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("runs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: /^runs$/i }).click();
    await expect(page).toHaveURL(/\/app\/runs/);
  });

  test("workflows link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: /workflows/i }).click();
    await expect(page).toHaveURL(/\/app\/workflows/);
  });
});
