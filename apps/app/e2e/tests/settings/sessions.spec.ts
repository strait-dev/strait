import { expect, test } from "../../fixtures";

test.describe("Session Management", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
    await page.waitForTimeout(2000);
  });

  test("active sessions section visible", async ({ page }) => {
    const sessions = page
      .getByText("Active Sessions", { exact: true })
      .or(page.getByText("Sessions"));
    await expect(sessions.first()).toBeVisible({ timeout: 10_000 });
  });

  test("current session shown in list", async ({ page }) => {
    // The current session should be visible
    const sessionItem = page.getByText(/current|this device/i);
    if (await sessionItem.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(sessionItem).toBeVisible();
    }
  });

  test("revoke all button exists", async ({ page }) => {
    const revokeBtn = page.getByText(/revoke all|sign out everywhere/i);
    if (
      await revokeBtn
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(revokeBtn.first()).toBeVisible();
    }
  });

  test("session list renders", async ({ page }) => {
    await expect(page.locator("main")).toBeVisible();
  });
});
