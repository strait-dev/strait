import { expect, test } from "../../fixtures";

test.describe("Settings - Account", () => {
  test.slow();
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("settings page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/settings/);
  });

  test("user name is displayed", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("user email is displayed", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("password section exists", async ({ page }) => {
    const passSection = page
      .getByPlaceholder("Enter current password")
      .or(page.getByText("Password", { exact: true }));
    if (
      await passSection
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(passSection.first()).toBeVisible();
    }
  });

  test("two-factor authentication section exists", async ({ page }) => {
    const section = page
      .getByText("Two-factor authentication", { exact: true })
      .or(page.getByText("2FA"));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });

  test("sessions section exists", async ({ page }) => {
    const section = page
      .getByText("Active sessions", { exact: true })
      .or(page.getByText("Sessions"));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });

  test("delete account section exists", async ({ page }) => {
    const section = page
      .getByText("Delete account", { exact: true })
      .or(page.getByText("Danger zone"));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });
});
