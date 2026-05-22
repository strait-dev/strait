import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let webhookId: string;
let webhookUrl: string;

test.describe("Webhook data flows", () => {
  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    webhookUrl = api.fakeEndpoint("/echo?name=data-webhook");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["run.completed", "run.failed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));
    webhookId = webhook.id;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("created webhook appears in the list and opens the detail page", async ({
    page,
  }) => {
    await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("table", { name: "Webhooks" })).toBeVisible();

    await expect(page.getByText(webhookUrl).first()).toBeVisible();
    await page.goto(`/app/webhooks/${webhookId}`, {
      waitUntil: "domcontentloaded",
    });

    await expect(page).toHaveURL(new RegExp(`/app/webhooks/${webhookId}`));
    await expect(page.getByText(webhookUrl).first()).toBeVisible();
  });

  test("deleted webhook is removed from the list", async ({ page }) => {
    const deleteUrl = api.fakeEndpoint("/echo?name=delete-webhook");
    const webhook = await api.createWebhook({
      webhook_url: deleteUrl,
      event_types: ["run.completed"],
    });

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(deleteUrl).first()
    );
    await selectTab(page, "Settings");
    await expect(page.getByText("Subscribed events")).toBeVisible();
    await api.deleteWebhook(webhook.id);
    await expect(async () => {
      const webhooks = await api.listWebhooks({ limit: 100 });
      expect(webhooks.data.some((item) => item.id === webhook.id)).toBe(false);
    }).toPass({ timeout: 15_000 });

    await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
    await expect(page.getByText(deleteUrl)).not.toBeVisible();
  });
});
