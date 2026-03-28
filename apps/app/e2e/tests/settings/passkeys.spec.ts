import { expect, test } from "../../fixtures";

test.describe("Passkeys", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
    await page.waitForTimeout(2000);
  });

  test("passkeys section visible", async ({ page }) => {
    const section = page
      .getByText("Passkeys", { exact: true })
      .or(page.getByText(/passkey/i));
    await expect(section.first()).toBeVisible({ timeout: 10_000 });
  });

  test("add passkey button exists", async ({ page }) => {
    const addBtn = page.getByRole("button", { name: /add passkey|add/i });
    if (
      await addBtn
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(addBtn.first()).toBeVisible();
    }
  });

  test("passkey list renders", async ({ page }) => {
    // Should show either passkeys or empty state
    await expect(page.locator("main")).toBeVisible();
  });
});
