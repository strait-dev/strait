import { test, expect } from "../../fixtures";

test.describe("Settings - Account", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("settings page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/settings/);
  });

  test("user name is displayed", async ({ page }) => {
    await expect(page.getByText(/E2E Test User|Leonardo/i)).toBeVisible({ timeout: 10_000 });
  });

  test("user email is displayed", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL ?? "test@example.com";
    await expect(page.getByText(email)).toBeVisible({ timeout: 10_000 });
  });

  test("password section exists", async ({ page }) => {
    await expect(page.getByText(/password/i)).toBeVisible();
  });

  test("two-factor authentication section exists", async ({ page }) => {
    await expect(page.getByText(/two-factor|2fa/i)).toBeVisible();
  });

  test("sessions section exists", async ({ page }) => {
    await expect(page.getByText(/sessions|active session/i)).toBeVisible();
  });

  test("delete account section exists", async ({ page }) => {
    await expect(page.getByText(/delete account|danger/i)).toBeVisible();
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/settings");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });
});
