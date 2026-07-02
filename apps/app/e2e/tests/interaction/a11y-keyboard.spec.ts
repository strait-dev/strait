import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Keyboard and accessibility interactions", () => {
  test.describe.configure({ timeout: 120_000 });

  test("exposes the command palette and primary navigation accessibly", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      "/app/dashboard",
      page.getByRole("button", { name: /search/i })
    );

    await expect(
      page.getByRole("heading", { name: "Command palette" })
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "Webhooks" })).toBeVisible();

    await page.getByRole("link", { name: "Webhooks" }).focus();
    await expect(page.getByRole("link", { name: "Webhooks" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/app\/webhooks/);
  });

  test("exposes accessible DLQ run detail controls", async ({ page }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const { run } = await data.deadLetterRun("keyboard-dlq");

    try {
      await gotoAndExpect(
        page,
        "/app/dlq",
        page.getByRole("table", { name: "Dead letter queue" })
      );
      const row = page
        .getByRole("table", { name: "Dead letter queue" })
        .locator("tbody tr")
        .filter({ hasText: run.id.slice(0, 8) })
        .first();
      await expect(row).toBeVisible();

      const viewRunButton = row.getByRole("button", {
        name: `View run ${run.id}`,
      });
      await expect(viewRunButton).toBeVisible();
      await viewRunButton.focus();
      await expect(viewRunButton).toBeFocused();
    } finally {
      await data.cleanup.run();
    }
  });

  test("keeps core dashboard landmarks and primary navigation reachable", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      "/app/dashboard",
      page.getByRole("heading", { name: "Dashboard" })
    );

    await expect(page.getByRole("main")).toBeVisible();
    await expect(page.getByRole("navigation").first()).toBeVisible();
    await expect(page.getByRole("link", { name: "Jobs" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Workflows" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Runs" })).toBeVisible();
  });
});
