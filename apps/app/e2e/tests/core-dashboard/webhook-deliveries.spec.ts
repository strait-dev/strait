import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect } from "../../support/navigation";
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

  test("shows delivered webhook attempts from real workflow events", async ({
    page,
  }) => {
    const webhookUrl = api.fakeEndpoint("/echo?name=webhook-delivery");
    const webhook = await api.createWebhook({
      webhook_url: webhookUrl,
      event_types: ["workflow.completed"],
    });
    data.cleanup.add(() => api.deleteWebhook(webhook.id));

    const job = await data.job("webhook-delivery-workflow");
    const workflow = await api.createWorkflow({
      name: data.name("webhook-delivery-workflow"),
      steps: [
        {
          job_id: job.id,
          step_ref: "deliver-webhook",
          depends_on: [],
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      source: "webhook-delivery-e2e",
    });
    await api.waitForWorkflowRunStatus(workflowRun.id, ["completed"], 90_000);
    await api.waitForWebhookDelivery(
      (delivery) =>
        delivery.subscription_id === webhook.id &&
        delivery.status === "delivered" &&
        delivery.last_status_code === 200,
      60_000
    );

    await gotoAndExpect(
      page,
      `/app/webhooks/${webhook.id}`,
      page.getByRole("table", { name: "Webhook deliveries" })
    );

    await expect(page.getByText(webhookUrl)).toBeVisible();
    await expect(page.getByRole("tab", { name: "Deliveries" })).toBeVisible();
    await expect(
      page.getByRole("row", { name: /Completed\s+200/ })
    ).toBeVisible();

    const deliveries = await api.listWebhookDeliveries({
      webhook_id: webhook.id,
      limit: 20,
    });
    expect(
      deliveries.data.some(
        (delivery) =>
          delivery.subscription_id === webhook.id &&
          delivery.status === "delivered" &&
          delivery.last_status_code === 200
      )
    ).toBe(true);
    await expect
      .poll(async () => (await api.getWorkflowRun(workflowRun.id)).status)
      .toBe("completed");
  });
});
