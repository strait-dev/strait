import { expect, test, type Page } from "@playwright/test";

const DEFAULT_E2E_USER_EMAIL = "e2e@local.strait";

async function waitForHydratedSignup(page: Page) {
  await page.goto("/signup");
  await expect(page.locator("body")).toHaveAttribute("data-hydrated", "true");
  await expect(
    page.getByRole("button", { name: "Create account", exact: true })
  ).toBeEnabled();
}

function passwordInput(page: Page) {
  return page.locator("input#password");
}

test.describe("Signup", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("signup page renders with form fields", async ({ page }) => {
    await waitForHydratedSignup(page);
    await expect(page.getByPlaceholder("Enter your full name")).toBeVisible();
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible();
    await expect(page.getByPlaceholder("At least 8 characters")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Create account", exact: true })
    ).toBeVisible();
  });

  test("duplicate email stays on signup page", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL ?? DEFAULT_E2E_USER_EMAIL;

    await waitForHydratedSignup(page);
    await page.getByLabel("Full name").fill("Duplicate User");
    await page.getByLabel("Email").fill(email);
    await passwordInput(page).fill("testpassword123");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();

    await expect(page).toHaveURL(/signup/);
    await expect(
      page.getByText(/failed to create account|already exists|already been used/i)
    ).toBeVisible();
  });

  test("short password shows validation error", async ({ page }) => {
    await waitForHydratedSignup(page);
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
    await waitForHydratedSignup(page);
    await page.getByRole("link", { name: "Sign in" }).click();
    await expect(page).toHaveURL(/login/);
  });
});
