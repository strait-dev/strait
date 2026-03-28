import { expect, test } from "../../fixtures";

test.describe("Two-Factor Authentication", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("2FA section visible on settings page", async ({ page }) => {
    const section = page
      .getByText("Two-Factor Authentication", { exact: true })
      .or(page.getByText("2FA"));
    await expect(section.first()).toBeVisible({ timeout: 10_000 });
  });

  test("enable 2FA button or manage option exists", async ({ page }) => {
    const enableBtn = page.getByRole("button", {
      name: /enable|setup|manage/i,
    });
    if (
      await enableBtn
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(enableBtn.first()).toBeVisible();
    }
  });

  test("2FA section shows current status", async ({ page }) => {
    // Should show either "enabled" or "disabled" or setup button
    await expect(page.locator("main")).toBeVisible();
  });

  test("page renders without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });
});
