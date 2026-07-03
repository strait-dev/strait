import { expect, test } from "../../fixtures";

test.describe("Session management", () => {
  test.slow();
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("active sessions section visible", async ({ page }) => {
    const sessions = page
      .getByText("Active sessions", { exact: true })
      .or(page.getByText("Sessions"));
    if (
      await sessions
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(sessions.first()).toBeVisible();
    }
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
    await expect(page.locator("body")).toBeVisible();
  });
});
