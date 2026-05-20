import { expect, test } from "../../fixtures";

test.describe("Contact form validation", () => {
  test("feedback form rejects oversized messages", async ({ page }) => {
    await page.goto("/app/dashboard");

    await page.getByRole("button", { name: "Send feedback" }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByText("Do you have any suggestions?")
    ).toBeVisible();

    await dialog.getByRole("combobox", { name: "Subject" }).click();
    await page.getByRole("option", { name: "Bug" }).click();
    await dialog.getByLabel("Message").fill("a".repeat(4001));
    await dialog.getByRole("button", { name: /^Send feedback/ }).click();

    await expect(dialog.getByText(/4000 characters or less/i)).toBeVisible();
  });

  test("support form rejects oversized problem details", async ({ page }) => {
    await page.goto("/app/dashboard");

    await page.getByRole("button", { name: "Get help" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Need help?")).toBeVisible();

    await dialog.getByRole("combobox", { name: "Subject" }).click();
    await page.getByRole("option", { name: "Technical Issue" }).click();
    await dialog.getByLabel("Problem Description").fill("a".repeat(4001));
    await dialog
      .getByLabel("Steps to Reproduce")
      .fill("Open the dashboard and inspect the workflow.");
    await dialog
      .getByLabel("Expected Result")
      .fill("The workflow should show the latest state.");
    await dialog
      .getByLabel("Actual Result")
      .fill("The workflow keeps showing the previous state.");
    await dialog.getByRole("button", { name: /^Send Request/ }).click();

    await expect(dialog.getByText(/4000 characters or less/i)).toBeVisible();
  });

  test("enterprise inquiry requires known options and bounded messages", async ({
    page,
  }) => {
    await page.goto("/app/enterprise-contact");

    await page.getByLabel("Name").fill("Jane Smith");
    await page.getByLabel("Email").fill("jane@example.com");
    await page.getByLabel("Company").fill("Acme Inc.");
    await page
      .getByLabel("Message")
      .fill("We need help planning a larger Strait deployment.");
    await page.getByRole("button", { name: "Send inquiry" }).click();

    await expect(page.getByText("Team size is required")).toBeVisible();

    await page.getByRole("combobox", { name: "Team size" }).click();
    await page.getByRole("option", { name: "51-200" }).click();
    await page.getByLabel("Message").fill("a".repeat(4001));
    await page.getByRole("button", { name: "Send inquiry" }).click();

    await expect(page.getByText(/4000 characters or less/i)).toBeVisible();
  });
});
