import { expect, test } from "@playwright/test";

test.describe("Signup", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("successful signup shows email verification page", async ({ page }) => {
    const uniqueEmail = `e2e-signup-${Date.now()}@test.example.com`;

    await page.goto("/signup");
    await page
      .getByPlaceholder("Enter your full name")
      .fill("Test Signup User");
    await page.getByPlaceholder("you@example.com").fill(uniqueEmail);
    await page
      .getByPlaceholder("At least 8 characters")
      .fill("testpassword123");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(page.getByText(/check your email|verification/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("duplicate email shows error", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL;
    if (!email) {
      test.skip();
      return;
    }

    await page.goto("/signup");
    await page.getByPlaceholder("Enter your full name").fill("Duplicate User");
    await page.getByPlaceholder("you@example.com").fill(email);
    await page
      .getByPlaceholder("At least 8 characters")
      .fill("testpassword123");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(
      page.getByText(/already exists|already registered|already in use/i)
    ).toBeVisible({ timeout: 10_000 });
  });

  test("short password shows validation error", async ({ page }) => {
    await page.goto("/signup");
    await page.getByPlaceholder("Enter your full name").fill("Short Pass");
    await page
      .getByPlaceholder("you@example.com")
      .fill("shortpass@test.example.com");
    await page.getByPlaceholder("At least 8 characters").fill("123");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(page.getByText(/at least 8 characters/i)).toBeVisible();
  });

  test("links to login page work", async ({ page }) => {
    await page.goto("/signup");
    await page.getByRole("link", { name: "Sign in" }).click();

    await expect(page).toHaveURL(/login/);
  });
});
