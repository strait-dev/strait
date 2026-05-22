import { expect, type Locator, test } from "@playwright/test";

test.describe("Login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("invalid credentials stay on the login page", async ({ page }) => {
    await page.goto("/login");
    await fillControlledInput(page.locator("#email"), "invalid@example.com");
    await fillControlledInput(page.locator("#password"), "wrongpassword123");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    // Should show error or stay on login page
    await expect(page).toHaveURL(/login/);
  });

  test("empty form stays on login page", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page).toHaveURL(/login/);
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("link", { name: /forgot/i }).click();
    await expect(page).toHaveURL(/forgot-password/);
  });
});

async function fillControlledInput(locator: Locator, value: string) {
  await locator.evaluate((el, nextValue) => {
    const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value"
    )?.set;
    nativeInputValueSetter?.call(el, nextValue);
    el.dispatchEvent(new Event("input", { bubbles: true }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
  }, value);
}
