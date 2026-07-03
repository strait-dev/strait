import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Webhook delivery dashboard", () => {
  test.describe.configure({ timeout: 240_000 });
  test.setTimeout(240_000);

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("shows webhook delivery history and subscription settings", async ({
    page,
  }) => {
    const webhookUrl = api.fakeEndpoint("/echo?name=webhook-delivery");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["workflow.completed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(webhookUrl).first()
    );

    await expect(page.getByText(webhookUrl).first()).toBeVisible();
    await expect(page.getByRole("tab", { name: "Deliveries" })).toBeVisible();
    await selectTab(page, "Settings");
    await expect(page.getByText("Subscribed events")).toBeVisible();
    await expect(page.getByText("workflow.completed")).toBeVisible();
  });

  test("shows failed webhook delivery attempts from the Go API", async ({
    page,
  }) => {
    const webhookUrl = api.fakeEndpoint("/status/400?name=webhook-failed");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["workflow.completed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));

    const seededDelivery = await api.seedWebhookDelivery({
      subscription_id: webhook.id,
      webhook_url: webhookUrl,
      status: "dead",
      attempts: 3,
      max_attempts: 3,
      last_status_code: 400,
      last_error: "client error: status 400",
      event_type: "workflow.completed",
    });
    data.cleanup.add(() => api.deleteWebhookDelivery(seededDelivery.id));

    const delivery = await api.waitForWebhookDelivery(
      (candidate) =>
        candidate.subscription_id === webhook.id &&
        (candidate.status === "failed" || candidate.status === "dead") &&
        candidate.last_status_code === 400,
      60_000
    );
    expect(delivery.last_error).toBeTruthy();

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(webhookUrl).first()
    );
    await selectTab(page, "Deliveries");
    await expect(
      page.getByRole("tabpanel", { name: "Deliveries" }).getByRole("table")
    ).toBeVisible();

    await expect(page.getByText(webhookUrl).first()).toBeVisible();
    await expect(page.getByRole("row", { name: /Failed\s+400/ })).toBeVisible();
  });

  test("shows failed delivery records created by real dispatch attempts", async ({
    page,
  }) => {
    const webhookUrl =
      process.env.E2E_WEBHOOK_FAILURE_URL ??
      api.fakeEndpoint("/status/400?name=webhook-real-dispatch");
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
        (candidate.status === "failed" ||
          candidate.status === "dead" ||
          candidate.status === "dead_letter") &&
        candidate.last_status_code === 400 &&
        candidate.attempts > 0,
      120_000
    );

    expect(delivery.last_error).toBeTruthy();
    expect(delivery.attempts).toBeGreaterThan(0);

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByText(webhookUrl).first()
    );
    await selectTab(page, "Deliveries");
    await expect(
      page.getByRole("tabpanel", { name: "Deliveries" }).getByRole("table")
    ).toBeVisible();
    await expect(page.getByText(webhookUrl).first()).toBeVisible();
    await expect(page.getByRole("row", { name: /Failed\s+400/ })).toBeVisible();
  });
});
