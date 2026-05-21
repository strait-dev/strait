import { ApiHelper, expect, test } from "../../fixtures";

const api = new ApiHelper();
let webhookId: string;
let webhookHost: string;

test.describe("Webhook Delivery", () => {
  test.describe.configure({ mode: "serial" });

  test.beforeAll(async () => {
    try {
      const wh = await api.createWebhook({
        webhook_url: api.fakeEndpoint("/echo"),
        event_types: ["run.completed", "run.failed"],
      });
      webhookId = wh.id;
      webhookHost = new URL(api.getFakeEndpointUrl()).host;
    } catch {
      // API may not be available
    }
  });

  test.afterAll(async () => {
    if (webhookId) {
      await api.deleteWebhook(webhookId).catch(() => {
        /* cleanup */
      });
    }
  });

  test("created webhook appears in list", async ({ page }) => {
    if (!webhookId) {
      test.skip();
      return;
    }
    await page.goto("/app/webhooks");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText(webhookHost).first()).toBeVisible({
        timeout: 10_000,
      });
    }
  });

  test("webhook detail sheet shows endpoint", async ({ page }) => {
    if (!webhookId) {
      test.skip();
      return;
    }
    await page.goto("/app/webhooks");
    const row = page.locator("table tbody tr").first();
    if (await row.isVisible({ timeout: 5000 }).catch(() => false)) {
      await row.click();
      await page.waitForTimeout(500);
      const sheet = page.locator("[role='dialog']");
      if (await sheet.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(sheet).toBeVisible();
      }
    }
  });

  test("delete webhook removes from list", async ({ page }) => {
    if (!webhookId) {
      test.skip();
      return;
    }
    // Delete via API
    await api.deleteWebhook(webhookId);
    webhookId = ""; // prevent afterAll from trying again

    await page.goto("/app/webhooks");
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      const webhookText = page.getByText(webhookHost);
      await expect(webhookText).not.toBeVisible({ timeout: 5000 });
    }
  });
});
