import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures";

const MAX_MESSAGE = "a".repeat(4000);
const OVERSIZED_MESSAGE = "a".repeat(4001);
const VALID_MESSAGE = "This message has enough detail to pass validation.";

async function selectOption(page: Page, name: string, option: string) {
  await page.getByRole("combobox", { name }).click();
  await page.getByRole("option", { name: option }).click();
}

async function openFeedbackDialog(page: Page) {
  await page.goto("/app/dashboard");
  await page.getByRole("button", { name: "Send feedback" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Do you have any suggestions?")).toBeVisible();
  return dialog;
}

async function openSupportDialog(page: Page) {
  await page.goto("/app/dashboard");
  await page.getByRole("button", { name: "Get help" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Need help?")).toBeVisible();
  return dialog;
}

test.describe("Contact form validation", () => {
  test("feedback form reports missing subject and short messages", async ({
    page,
  }) => {
    const dialog = await openFeedbackDialog(page);

    await dialog.getByLabel("Message").fill("short");
    await dialog.getByRole("button", { name: /^Send feedback/ }).click();

    await expect(dialog.getByText("You must choose a subject")).toBeVisible();
    await expect(
      dialog.getByText("Message must be at least 10 characters")
    ).toBeVisible();
  });

  test("feedback form accepts the message limit and rejects overflow", async ({
    page,
  }) => {
    const dialog = await openFeedbackDialog(page);

    await selectOption(page, "Subject", "Bug");
    await dialog.getByLabel("Message").fill(MAX_MESSAGE);
    await expect(
      dialog.getByText(/4000 characters or less/i)
    ).not.toBeVisible();

    await dialog.getByLabel("Message").fill(OVERSIZED_MESSAGE);
    await dialog.getByRole("button", { name: /^Send feedback/ }).click();

    await expect(dialog.getByText(/4000 characters or less/i)).toBeVisible();
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
    const dialog = await openSupportDialog(page);

    await dialog.getByLabel("Problem Description").fill("short");
    await dialog.getByLabel("Steps to Reproduce").fill("short");
    await dialog.getByLabel("Expected Result").fill("short");
    await dialog.getByLabel("Actual Result").fill("short");
    await dialog.getByRole("button", { name: /^Send Request/ }).click();

    await expect(dialog.getByText("Select a subject")).toBeVisible();
    await expect(
      dialog.getByText("Message must be at least 10 characters")
    ).toBeVisible();
    await expect(
      dialog.getByText("Describe the steps to reproduce the problem")
    ).toBeVisible();
    await expect(
      dialog.getByText("Describe the expected result")
    ).toBeVisible();
    await expect(dialog.getByText("Describe the actual result")).toBeVisible();
  });

  test("support form rejects oversized details for every long field", async ({
    page,
  }) => {
    const dialog = await openSupportDialog(page);

    await selectOption(page, "Subject", "Technical Issue");
    await selectOption(page, "Priority", "High");
    await selectOption(page, "Environment", "Staging");
    await dialog.getByLabel("Problem Description").fill(OVERSIZED_MESSAGE);
    await dialog.getByLabel("Steps to Reproduce").fill(OVERSIZED_MESSAGE);
    await dialog.getByLabel("Expected Result").fill(OVERSIZED_MESSAGE);
    await dialog.getByLabel("Actual Result").fill(OVERSIZED_MESSAGE);
    await dialog.getByRole("button", { name: /^Send Request/ }).click();

    await expect(
      dialog.getByText("Message must be 4000 characters or less")
    ).toBeVisible();
    await expect(
      dialog.getByText("Steps must be 4000 characters or less")
    ).toBeVisible();
    await expect(
      dialog.getByText("Expected result must be 4000 characters or less")
    ).toBeVisible();
    await expect(
      dialog.getByText("Actual result must be 4000 characters or less")
    ).toBeVisible();
  });

  test("support cooldown disables the launcher", async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("support_cooldown", String(Date.now() + 60_000));
    });

    await page.goto("/app/dashboard");

    await expect(page.getByRole("button", { name: "Get help" })).toBeDisabled();
  });

  test("enterprise inquiry reports required team size and invalid email", async ({
    page,
  }) => {
    await page.goto("/app/enterprise-contact");

    await page.getByLabel("Name").fill("Jane Smith");
    await page.getByLabel("Email").fill("not-an-email");
    await page.getByLabel("Company").fill("Acme Inc.");
    await page.getByLabel("Message").fill(VALID_MESSAGE);
    await page.getByRole("button", { name: "Send inquiry" }).click();

    await expect(page.getByText("Must be a valid email address")).toBeVisible();
    await expect(page.getByText("Team size is required")).toBeVisible();
  });

  test("enterprise inquiry rejects overlong identity and message fields", async ({
    page,
  }) => {
    await page.goto("/app/enterprise-contact");

    await page.getByLabel("Name").fill("a".repeat(121));
    await page.getByLabel("Email").fill("jane@example.com");
    await page.getByLabel("Company").fill("a".repeat(121));
    await selectOption(page, "Team size", "51-200");
    await selectOption(page, "Primary use case (optional)", "Data residency");
    await selectOption(
      page,
      "Expected monthly spend (optional)",
      "$500 - $1,500/mo"
    );
    await page.getByLabel("Message").fill(OVERSIZED_MESSAGE);
    await page.getByRole("button", { name: "Send inquiry" }).click();

    await expect(
      page.getByText("Name must be 120 characters or less")
    ).toBeVisible();
    await expect(
      page.getByText("Company name must be 120 characters or less")
    ).toBeVisible();
    await expect(
      page.getByText("Message must be 4000 characters or less")
    ).toBeVisible();
  });
});
