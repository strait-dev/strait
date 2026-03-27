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
    await page.getByPlaceholder("you@example.com").fill(email);
    await page.getByPlaceholder("Enter your password").fill(password);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page).toHaveURL(/\/app/);
  });

  test("invalid credentials shows error message", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("you@example.com").fill("invalid@example.com");
    await page.getByPlaceholder("Enter your password").fill("wrongpassword123");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page.getByText(/invalid|incorrect|not found/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("empty form shows validation errors", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page.getByText(/email|required/i)).toBeVisible();
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("link", { name: /forgot/i }).click();

    await expect(page).toHaveURL(/forgot-password/);
  });
});
