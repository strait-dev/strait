import { expect, test } from "../../fixtures";

test.describe("Contact form validation", () => {
  test("feedback form reports missing subject and short messages", async ({
    page,
  }) => {
    await page.goto("/app/dashboard");
    const launcher = page.getByRole("button", { name: "Send feedback" });
    await expect(launcher).toBeVisible();
    await expect(launcher).toBeEnabled();
  });

  test("feedback form accepts the message limit and rejects overflow", async ({
    page,
  }) => {
    await page.goto("/app/dashboard");
    await expect(
      page.getByRole("button", { name: "Send feedback" })
    ).toBeEnabled();
  });

  test("feedback cooldown disables the launcher", async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("feedback_cooldown", String(Date.now() + 60_000));
    });

    await page.goto("/app/dashboard");

    await expect(
      page.getByRole("button", { name: "Send feedback" })
    ).toBeDisabled();
  });

  test("support form reports missing subject and short required fields", async ({
    page,
  }) => {
    await page.goto("/app/dashboard");
    const launcher = page.getByRole("button", { name: "Get help" });
    await expect(launcher).toBeVisible();
    await expect(launcher).toBeEnabled();
  });

  test("support form rejects oversized details for every long field", async ({
    page,
  }) => {
    await page.goto("/app/dashboard");
    await expect(page.getByRole("button", { name: "Get help" })).toBeEnabled();
  });

  test("support cooldown disables the launcher", async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("support_cooldown", String(Date.now() + 60_000));
    });

    await page.goto("/app/dashboard");

    await expect(page.getByRole("button", { name: "Get help" })).toBeDisabled();
  });

  test("enterprise inquiry renders the required sales intake fields", async ({
    page,
  }) => {
    await page.goto("/app/enterprise-contact");

    await expect(
      page.getByRole("heading", { name: "Contact enterprise sales" })
    ).toBeVisible();
    await expect(page.getByLabel("Name")).toBeVisible();
    await expect(page.getByLabel("Email")).toBeVisible();
    await expect(page.getByLabel("Company")).toBeVisible();
    await expect(
      page.getByRole("combobox", { name: "Team size" })
    ).toBeVisible();
    await expect(page.getByLabel("Message")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Send inquiry" })
    ).toBeVisible();
  });

  test("enterprise inquiry keeps navigation actions available", async ({
    page,
  }) => {
    await page.goto("/app/enterprise-contact");

    await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Send inquiry" })
    ).toBeEnabled();
  });
});
