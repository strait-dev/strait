import { expect, test } from "../../fixtures";

test.describe("Sidebar Navigation", () => {
  test("dashboard link navigates correctly", async ({ page }) => {
    await page.goto("/app/jobs");
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page).toHaveURL(/\/app\/dashboard/);
  });

  test("jobs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: "Jobs" }).click();
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("runs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: "Runs" }).click();
    await expect(page).toHaveURL(/\/app\/runs/);
  });

  test("workflows link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.getByRole("link", { name: "Workflows" }).click();
    await expect(page).toHaveURL(/\/app\/workflows/);
  });
});
