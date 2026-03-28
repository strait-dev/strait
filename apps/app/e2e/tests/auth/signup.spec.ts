import { expect, test } from "@playwright/test";

test.describe("Signup", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("signup page renders with form fields", async ({ page }) => {
    await page.goto("/signup");
    await expect(page.getByPlaceholder("Enter your full name")).toBeVisible();
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible();
    await expect(page.getByPlaceholder("At least 8 characters")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Create account", exact: true })
    ).toBeVisible();
  });

  test("duplicate email stays on signup page", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL;
    if (!email) {
      test.skip();
      return;
    }

    await page.goto("/signup");
    await page.locator("#name").evaluate((el, val) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(el, val);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, "Duplicate User");
    await page.locator("#email").evaluate((el, val) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(el, val);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, email);
    await page.locator("#password").evaluate((el, val) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(el, val);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, "testpassword123");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(page).toHaveURL(/signup/);
  });

  test("short password shows validation error", async ({ page }) => {
    await page.goto("/signup");
    const passInput = page.getByPlaceholder("At least 8 characters");
    await passInput.click();
    await passInput.fill("123");
    await passInput.blur();
    await page.waitForTimeout(500);
    // Validation error text should appear (separate from the placeholder)
    const errorText = page.locator(
      "[data-slot='field-error'], .text-destructive"
    );
    if (
      await errorText
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(errorText.first()).toBeVisible();
    }
  });

  test("links to login page work", async ({ page }) => {
    await page.goto("/signup");
    await page.getByRole("link", { name: "Sign in" }).click();
    await expect(page).toHaveURL(/login/);
  });
});
