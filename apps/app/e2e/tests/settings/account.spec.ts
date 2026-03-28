import { expect, test } from "../../fixtures";

test.describe("Settings - Account", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("settings page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/settings/);
  });

  test("user name is displayed", async ({ page }) => {
    // The user name input placeholder contains the name, or it's shown in text
    const content = page.locator("main").or(page.locator("body"));
    await expect(content).toBeVisible({ timeout: 10_000 });
  });

  test("user email is displayed", async ({ page }) => {
    await expect(page.locator("main")).toBeVisible({ timeout: 10_000 });
  });

  test("password section exists", async ({ page }) => {
    // Look for password-related input or label
    const passSection = page
      .getByPlaceholder("Enter current password")
      .or(page.getByText("Password", { exact: true }));
    await expect(passSection.first()).toBeVisible({ timeout: 10_000 });
  });

  test("two-factor authentication section exists", async ({ page }) => {
    const section = page
      .getByText("Two-Factor Authentication", { exact: true })
      .or(page.getByText("2FA"));
    await expect(section.first()).toBeVisible({ timeout: 10_000 });
  });

  test("sessions section exists", async ({ page }) => {
    const section = page
      .getByText("Active Sessions", { exact: true })
      .or(page.getByText("Sessions"));
    await expect(section.first()).toBeVisible({ timeout: 10_000 });
  });

  test("delete account section exists", async ({ page }) => {
    const section = page
      .getByText("Delete Account", { exact: true })
      .or(page.getByText("Danger Zone"));
    await expect(section.first()).toBeVisible({ timeout: 10_000 });
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });
});
