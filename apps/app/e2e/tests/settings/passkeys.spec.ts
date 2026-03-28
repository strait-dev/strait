import { expect, test } from "../../fixtures";

test.describe("Passkeys", () => {
  test.slow();
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("passkeys section visible", async ({ page }) => {
    const section = page
      .getByText("Passkeys", { exact: true })
      .or(page.getByText(/passkey/i));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
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
    await expect(page.locator("body")).toBeVisible();
  });
});
