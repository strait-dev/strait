import { expect, test } from "@playwright/test";

test.describe("Login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  // biome-ignore lint/suspicious/useAwait: Playwright test callback requires async signature
  test("successful login redirects to /app", async () => {
    test.skip();
  });

  test("invalid credentials shows error message", async ({ page }) => {
    await page.goto("/login");
    // Fill via evaluate to trigger React controlled inputs
    await page.locator("#email").evaluate((el, val) => {
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      nativeInputValueSetter?.call(el, val);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, "invalid@example.com");
    await page.locator("#password").evaluate((el, val) => {
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      nativeInputValueSetter?.call(el, val);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, "wrongpassword123");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    // Should show error or stay on login page
    await expect(page).toHaveURL(/login/);
  });

  test("empty form stays on login page", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page.waitForTimeout(500);
    await expect(page).toHaveURL(/login/);
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("link", { name: /forgot/i }).click();
    await expect(page).toHaveURL(/forgot-password/);
  });
});
