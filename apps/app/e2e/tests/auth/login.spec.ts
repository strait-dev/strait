import { expect, test, type Page } from "@playwright/test";

const DEFAULT_E2E_USER_EMAIL = "e2e@local.strait";
const DEFAULT_E2E_USER_PASSWORD = "e2epassword123";

async function waitForHydratedLogin(page: Page) {
  await page.goto("/login");
  await expect(page.locator("body")).toHaveAttribute("data-hydrated", "true");
  await expect(
    page.getByRole("button", { name: "Sign in", exact: true })
  ).toBeEnabled();
}

function passwordInput(page: Page) {
  return page.locator("input#password");
}

test.describe("Login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("successful login redirects to /app", async ({ page }) => {
    const email = process.env.E2E_USER_EMAIL ?? DEFAULT_E2E_USER_EMAIL;
    const password =
      process.env.E2E_USER_PASSWORD ?? DEFAULT_E2E_USER_PASSWORD;

    await waitForHydratedLogin(page);
    await page.getByLabel("Email").fill(email);
    await passwordInput(page).fill(password);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await page.waitForURL("**/app", { timeout: 15_000 });
    await expect(page).toHaveURL(/\/app$/);
  });

  test("invalid credentials shows error message", async ({ page }) => {
    await waitForHydratedLogin(page);
    await page.getByLabel("Email").fill("invalid@example.com");
    await passwordInput(page).fill("wrongpassword123");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    await expect(page).toHaveURL(/login/);
    await expect(
      page.getByText(/invalid email or password/i).first()
    ).toBeVisible();
  });

  test("empty form stays on login page", async ({ page }) => {
    await waitForHydratedLogin(page);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page).toHaveURL(/login/);
    await expect(page.getByText(/invalid email address/i)).toBeVisible();
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await waitForHydratedLogin(page);
    await page.getByRole("link", { name: /forgot/i }).click();
    await expect(page).toHaveURL(/forgot-password/);
  });
});
