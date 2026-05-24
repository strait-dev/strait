import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Webhook delivery lifecycle", () => {
  test.describe.configure({ timeout: 240_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("records successful real dispatch attempts as delivered rows", async ({
    page,
  }) => {
    const webhookUrl =
      process.env.E2E_WEBHOOK_SUCCESS_URL ??
      api.fakeEndpoint("/success?name=webhook-real-success");
    const redactedWebhookUrl = new URL(webhookUrl).origin;
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["workflow.completed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));

    const pendingDelivery = await api.seedPendingWebhookDelivery({
      subscription_id: webhook.id,
      webhook_url: webhookUrl,
      event_type: "workflow.completed",
    });
    data.cleanup.add(() => api.deleteWebhookDelivery(pendingDelivery.id));

    const delivery = await api.waitForWebhookDelivery(
      (candidate) =>
        candidate.id === pendingDelivery.id &&
        candidate.subscription_id === webhook.id &&
        candidate.webhook_url === redactedWebhookUrl &&
        candidate.status === "delivered" &&
        candidate.last_status_code === 200 &&
        candidate.attempts > 0,
      120_000
    );
    expect(delivery.attempts).toBeGreaterThan(0);

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(webhookUrl).first()
    );
    await selectTab(page, "Deliveries");
    await expect(
      page.getByRole("table", { name: "Webhook deliveries" })
    ).toBeVisible();
    await expect(
      page.getByRole("row", { name: /Completed\s+200/ })
    ).toBeVisible();
  });

  test("renders dead deliveries at max attempts without treating them as pending", async ({
    page,
  }) => {
    const webhookUrl = api.fakeEndpoint("/status/500?name=webhook-dead");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["workflow.failed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));

    const deadDelivery = await api.seedWebhookDelivery({
      subscription_id: webhook.id,
      webhook_url: webhookUrl,
      status: "dead",
      attempts: 3,
      max_attempts: 3,
      last_status_code: 500,
      last_error: "server error: status 500",
      event_type: "workflow.failed",
    });
    data.cleanup.add(() => api.deleteWebhookDelivery(deadDelivery.id));

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(webhookUrl).first()
    );
    await selectTab(page, "Deliveries");
    const deliveriesTable = page.getByRole("table", {
      name: "Webhook deliveries",
    });
    await expect(deliveriesTable).toBeVisible();
    await expect(
      deliveriesTable.getByRole("row", { name: /Failed\s+500\s+3\/3/ })
    ).toBeVisible();
  });

  test("removes deleted subscriptions from the dashboard list", async ({
    page,
  }) => {
    const webhookUrl = api.fakeEndpoint("/echo?name=webhook-delete");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["run.completed"],
    });

    await gotoAndExpect(
      page,
      "/app/webhooks",
      page.getByRole("table", { name: "Webhooks" })
    );
    await expect(page.getByText(webhookUrl)).toBeVisible();

    await api.deleteWebhook(webhook.id);
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByText(webhookUrl)).toHaveCount(0);

    const subscriptions = await api.listWebhooks({ limit: 100 });
    expect(subscriptions.data.map((sub) => sub.id)).not.toContain(webhook.id);
  });
});
