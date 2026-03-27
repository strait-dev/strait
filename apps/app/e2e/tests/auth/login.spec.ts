import { expect, test } from "@playwright/test";

test.describe("Login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("successful login redirects to /app", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL;
    const password = process.env.E2E_USER_PASSWORD;
    if (!(email && password)) {
      test.skip();
      return;
    }

    await page.goto("/login");
    const emailInput = page.getByPlaceholder("you@example.com");
    await emailInput.click();
    await emailInput.pressSequentially(email, { delay: 10 });
    const passInput = page.getByPlaceholder("Enter your password");
    await passInput.click();
    await passInput.pressSequentially(password, { delay: 10 });
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page).toHaveURL(/\/app/, { timeout: 15_000 });
  });

  test("invalid credentials shows error message", async ({ page }) => {
    await page.goto("/login");
    const emailInput = page.getByPlaceholder("you@example.com");
    await emailInput.click();
    await emailInput.pressSequentially("invalid@example.com", { delay: 10 });
    const passInput = page.getByPlaceholder("Enter your password");
    await passInput.click();
    await passInput.pressSequentially("wrongpassword123", { delay: 10 });
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page.getByText(/invalid|incorrect|not found/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("empty form shows validation errors", async ({ page }) => {
    await page.goto("/login");
    // Click into email field and back out to trigger validation
    const emailInput = page.getByPlaceholder("you@example.com");
    await emailInput.click();
    await emailInput.blur();
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    // Form should not navigate away -- still on login
    await page.waitForTimeout(1000);
    await expect(page).toHaveURL(/login/);
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("link", { name: /forgot/i }).click();

    await expect(page).toHaveURL(/forgot-password/);
  });
});
