import { ApiHelper, expect, test } from "../../fixtures";
import { selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Organization API keys", () => {
  test.describe.configure({ timeout: 90_000 });
  test.setTimeout(90_000);

  test("creates and revokes an API key from organization settings", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const keyName = data.name("api-key");

    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await selectTab(page, "API Keys");
    await expect(page.getByText("Manage API keys")).toBeVisible();

    await page.getByRole("button", { name: "Create key" }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Create API Key" })
    ).toBeVisible();
    await dialog.getByLabel("Key Name").fill(keyName);
    await dialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "30 days" }).click();
    await dialog.getByRole("button", { name: "Create key" }).click();

    await expect(
      dialog.getByRole("heading", { name: "API key created" })
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      dialog.getByText(/won't be able to see it again/i)
    ).toBeVisible();
    await dialog.getByRole("button", { name: "Done" }).click();

    const row = page.getByRole("row", { name: new RegExp(keyName) });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await row.getByRole("button", { name: "Revoke" }).click();
    await page.getByRole("button", { name: "Revoke key" }).click();
    await expect(row).not.toBeVisible({ timeout: 15_000 });
  });
});
