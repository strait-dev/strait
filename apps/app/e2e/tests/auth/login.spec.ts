import { expect, type Locator, type Page, test } from "@playwright/test";
import { watchForRouteCrashes } from "../../support/route-crashes";

test.describe("Login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("valid credentials redirect to the app without route chunk crashes", async ({
    page,
  }) => {
    const email = process.env.E2E_USER_EMAIL;
    const password = process.env.E2E_USER_PASSWORD;
    if (!(email && password)) {
      throw new Error("E2E_USER_EMAIL and E2E_USER_PASSWORD are required");
    }

    const routeCrashes = watchForRouteCrashes(page);

    await page.goto("/login");
    await waitForClientRouter(page);
    await fillAndSubmitSignInForm(page, email, password);
    await expect(page).toHaveURL(/\/app/, { timeout: 30_000 });
    await expect(page.getByText("Project", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    routeCrashes.assertNoCrashes();
  });

  test("invalid credentials stay on the login page", async ({ page }) => {
    await page.goto("/login");
    await waitForClientRouter(page);
    await fillControlledInput(page.locator("#email"), "invalid@example.com");
    await fillControlledInput(page.locator("#password"), "wrongpassword123");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();

    // Should show error or stay on login page
    await expect(page).toHaveURL(/login/);
  });

  test("empty form stays on login page", async ({ page }) => {
    await page.goto("/login");
    await waitForClientRouter(page);
    await expect(
      page.getByRole("button", { name: "Sign in", exact: true })
    ).toBeDisabled();
    await expect(page).toHaveURL(/login/);
  });

  test("valid email and password keep sign in enabled", async ({ page }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.locator("#email").fill("e2e-owner@example.com");
    await page.locator("#password").fill("dogfood-local-password");

    const signInButton = page.locator("form button[type=submit]");
    await expect(signInButton).toContainText("Sign in");
    await expect(signInButton).toBeEnabled();
  });

  test("sign in button shows loading while submitting", async ({ page }) => {
    let interceptedSignIn = false;
    await page.route("**/api/auth/sign-in/email**", async (route) => {
      interceptedSignIn = true;
      await new Promise((resolve) => setTimeout(resolve, 1500));
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ error: { message: "Invalid credentials" } }),
      });
    });

    await page.goto("/login");
    await waitForClientRouter(page);
    await fillControlledInput(page.locator("#email"), "invalid@example.com");
    await fillControlledInput(page.locator("#password"), "wrongpassword123");

    const signInButton = page.locator("form button[type=submit]");
    await expect(signInButton).toContainText("Sign in");
    const click = signInButton.click();
    await expect(signInButton).toBeDisabled();
    await expect(signInButton.locator("svg")).toBeVisible();
    await click;
    expect(interceptedSignIn).toBe(true);
  });

  test("does not expose SSO while it is not configured", async ({ page }) => {
    await page.goto("/login");
    await waitForClientRouter(page);

    await expect(page.getByRole("link", { name: /sso roadmap/i })).toHaveCount(
      0
    );
  });

  test("forgot password link navigates correctly", async ({ page }) => {
    await page.goto("/login");
    await waitForClientRouter(page);
    await page.getByRole("link", { name: /forgot/i }).click();
    await expect(page).toHaveURL(/forgot-password/);
  });
});

async function fillAndSubmitSignInForm(
  page: Page,
  email: string,
  password: string
) {
  const emailInput = page.locator("#email");
  const passwordInput = page.locator("#password");
  const signInButton = page.getByRole("button", {
    name: "Sign in",
    exact: true,
  });

  await expect(
    page.getByRole("button", { name: "Sign in with magic link" })
  ).toBeVisible();

  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      await fillControlledInput(emailInput, email);
      await fillControlledInput(passwordInput, password);
      await expect(emailInput).toHaveValue(email, { timeout: 1000 });
      await expect(passwordInput).toHaveValue(password, { timeout: 1000 });
      await expect(signInButton).toBeEnabled({ timeout: 1000 });
      await signInButton.click({ timeout: 1000 });
      return;
    } catch {
      // The login route can finish a client remount after hydration in dev.
      // Re-fill the current fields and try the current submit button again.
    }
  }

  await signInButton.click();
}

async function fillControlledInput(locator: Locator, value: string) {
  await locator.click();
  await locator.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await locator.pressSequentially(value);
}

async function waitForClientRouter(page: Page) {
  await page.waitForFunction(
    () => Boolean((globalThis as { __TSR_ROUTER__?: unknown }).__TSR_ROUTER__),
    null,
    { timeout: 10_000 }
  );
}
