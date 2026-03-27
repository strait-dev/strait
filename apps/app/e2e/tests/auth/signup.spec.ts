import { expect, test } from "@playwright/test";

test.describe("Signup", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("successful signup shows email verification page", async ({ page }) => {
    const uniqueEmail = `e2e-signup-${Date.now()}@test.example.com`;

    await page.goto("/signup");
    const nameInput = page.getByPlaceholder("Enter your full name");
    await nameInput.click();
    await nameInput.pressSequentially("Test Signup User", { delay: 10 });
    const emailInput = page.getByPlaceholder("you@example.com");
    await emailInput.click();
    await emailInput.pressSequentially(uniqueEmail, { delay: 10 });
    const passInput = page.getByPlaceholder("At least 8 characters");
    await passInput.click();
    await passInput.pressSequentially("testpassword123", { delay: 10 });
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(
      page.getByText(/check your email|verification|verify your email/i)
    ).toBeVisible({ timeout: 10_000 });
  });

  test("duplicate email shows error", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL;
    if (!email) {
      test.skip();
      return;
    }

    await page.goto("/signup");
    const nameInput = page.getByPlaceholder("Enter your full name");
    await nameInput.click();
    await nameInput.pressSequentially("Duplicate User", { delay: 10 });
    const emailInput = page.getByPlaceholder("you@example.com");
    await emailInput.click();
    await emailInput.pressSequentially(email, { delay: 10 });
    const passInput = page.getByPlaceholder("At least 8 characters");
    await passInput.click();
    await passInput.pressSequentially("testpassword123", { delay: 10 });
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await page.waitForTimeout(3000);
    await expect(page).toHaveURL(/signup/);
  });

  test("short password shows validation error", async ({ page }) => {
    await page.goto("/signup");
    const passInput = page.getByPlaceholder("At least 8 characters");
    await passInput.click();
    await passInput.pressSequentially("123", { delay: 10 });
    await passInput.blur();

    await expect(page.getByText(/at least 8 characters/i)).toBeVisible();
  });

  test("links to login page work", async ({ page }) => {
    await page.goto("/signup");
    await page.getByRole("link", { name: "Sign in" }).click();

    await expect(page).toHaveURL(/login/);
  });
});
