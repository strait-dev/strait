import type { ApiHelper } from "../../fixtures";
import { expect, test } from "../../fixtures";
import { gotoAndExpect } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood operations journeys", () => {
  test.describe.configure({ timeout: 180_000 });

  test("creates, searches, views, and deletes a webhook through the dashboard", async ({
    api,
    page,
  }) => {
    const webhookName = `dogfood-webhook-${Date.now()}`;
    const webhookUrl = api.fakeEndpoint(`/echo?name=${webhookName}`);
    let webhookId: string | null = null;

    try {
      await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
      await page.getByRole("button", { name: "Create webhook" }).click();
      await page.getByLabel("URL").fill(webhookUrl);
      await page.getByText("Run completed").click();
      await expect(
        page.getByRole("checkbox", { name: "Run completed" })
      ).toBeChecked();
      await page.getByRole("button", { name: "Create webhook" }).last().click();

      await expect(
        page.getByText("Webhook created", { exact: true })
      ).toBeVisible({
        timeout: 15_000,
      });
      webhookId = await waitForWebhookIdByUrl(api, webhookUrl);

      await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
      await page.getByLabel("Search").fill(webhookName);
      const row = page.getByRole("row", { name: new RegExp(webhookName) });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row).toContainText("run.completed");

      await row.getByRole("link", { name: /View webhook/ }).click();
      await expect(page).toHaveURL(new RegExp(`/app/webhooks/${webhookId}`));
      await expect(
        page.locator("main").getByText(webhookUrl).first()
      ).toBeVisible();

      await api.clearFakeEndpointRequests();
      const sendTestButton = page.getByRole("button", { name: "Send test" });
      await expect(sendTestButton).toBeEnabled();
      await sendTestButton.click();
      await expect
        .poll(
          async () =>
            (
              await api.listFakeEndpointRequests({ name: webhookName })
            ).data.find((request) => request.path === "/echo")?.body,
          { timeout: 30_000 }
        )
        .toMatchObject({ type: "webhook.test" });

      await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
      await page.getByLabel("Search").fill(webhookName);
      const deleteRow = page.getByRole("row", {
        name: new RegExp(webhookName),
      });
      const deleteButton = deleteRow.getByRole("button", { name: "Delete" });
      await expect(deleteButton).toBeEnabled();
      await deleteButton.click();
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "Delete" })
        .click();

      await expect
        .poll(
          async () => (await api.listWebhooks({ search: webhookName })).data
        )
        .toHaveLength(0);
      webhookId = null;
    } finally {
      if (webhookId) {
        await api.deleteWebhook(webhookId).catch(() => undefined);
      }
    }
  });

  test("inspects and discards a dead-letter run through the dashboard", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const { job, run } = await data.deadLetterRun("dogfood-dlq");

    try {
      await gotoAndExpect(
        page,
        "/app/dlq",
        page.getByRole("region", { name: "Dead letter queue" })
      );
      await page.getByLabel("Search").fill(job.name);
      const row = page.getByRole("row", {
        name: new RegExp(run.id.slice(0, 8)),
      });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row).toContainText(job.id.slice(0, 8));

      const viewRunButton = row.getByRole("button", {
        name: `View run ${run.id}`,
      });
      await expect(viewRunButton).toBeEnabled();
      await viewRunButton.click();
      await expect(page.getByTestId("run-detail-sheet")).toContainText(run.id);
      await page.keyboard.press("Escape");

      await row.getByRole("checkbox", { name: "Select row" }).check();
      await page
        .getByRole("button", { name: /Discard selected \(1\)/ })
        .click();
      await page.getByRole("button", { name: "Discard" }).click();

      await expect
        .poll(async () => (await api.listDlqEntries({ limit: 100 })).data)
        .not.toContainEqual(expect.objectContaining({ id: run.id }));
    } finally {
      await data.cleanup.run();
    }
  });

  test("retries a dead-letter run from row actions", async ({ api, page }) => {
    const data = new TestDataFactory(api);
    const { job, run } = await data.deadLetterRun("dogfood-dlq-retry");

    try {
      await gotoAndExpect(
        page,
        "/app/dlq",
        page.getByRole("region", { name: "Dead letter queue" })
      );
      await page.getByLabel("Search").fill(job.name);
      const row = page.getByRole("row", {
        name: new RegExp(run.id.slice(0, 8)),
      });
      await expect(row).toBeVisible({ timeout: 15_000 });

      const retryButton = row.getByRole("button", { name: "Retry" });
      await expect(retryButton).toBeEnabled();
      await retryButton.click();

      await expect
        .poll(
          async () =>
            (await api.listDlqEntries({ limit: 100 })).data.some(
              (entry) => entry.id === run.id
            ),
          { timeout: 15_000 }
        )
        .toBe(false);
      await clearRunsForJob(api, job.id);
    } finally {
      await data.cleanup.run();
    }
  });

  test("surfaces event and log activity with search and status filters", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const eventKey = data.name("dogfood-event");
    const workflow = await api.createWorkflow({
      name: data.name("dogfood-event-workflow"),
      steps: [
        {
          step_ref: "await-dogfood-event",
          step_type: "wait_for_event",
          event_key: eventKey,
          event_timeout_secs: 300,
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    try {
      const workflowRun = await api.triggerWorkflow(workflow.id, {
        source: "dogfood-operations",
        event_key: eventKey,
      });
      data.cleanup.add(() =>
        api.sendEvent(eventKey, { cleanup: true }).catch(() => undefined)
      );
      await api.waitForEventTrigger(
        (event) =>
          event.event_key === eventKey &&
          event.workflow_run_id === workflowRun.id &&
          event.status === "waiting",
        60_000
      );

      await gotoAndExpect(
        page,
        "/app/events",
        page.getByRole("region", { name: "Events" })
      );
      await page.getByLabel("Search").fill(eventKey);
      await expect(page.getByText(eventKey)).toBeVisible({ timeout: 15_000 });
      await page.goto(
        `/app/events?status=waiting&query=${encodeURIComponent(eventKey)}`,
        { waitUntil: "domcontentloaded" }
      );
      const waitingRow = page.getByRole("row", { name: new RegExp(eventKey) });
      await expect(waitingRow).toBeVisible();
      await expect(waitingRow).toContainText("Waiting");

      await api.sendEvent(eventKey, { approved: true });
      await api.waitForEventTrigger(
        (event) =>
          event.event_key === eventKey &&
          event.workflow_run_id === workflowRun.id &&
          event.status === "received",
        60_000
      );

      await gotoAndExpect(
        page,
        "/app/logs?statuses=received",
        page.getByRole("region", { name: "Logs" })
      );
      await page.getByLabel("Search").fill(eventKey);
      const logRow = page.getByRole("row", { name: new RegExp(eventKey) });
      await expect(logRow).toBeVisible({ timeout: 15_000 });
      await expect(logRow).toContainText("workflow_step");
      await expect(logRow).toContainText("event");
    } finally {
      await data.cleanup.run();
    }
  });
});

async function waitForWebhookIdByUrl(api: ApiHelper, webhookUrl: string) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const webhooks = await api.listWebhooks({ limit: 100 });
    const match = webhooks.data.find(
      (webhook) => webhook.webhook_url === webhookUrl
    );
    if (match?.id) {
      return match.id;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Webhook ${webhookUrl} was not created`);
}

async function clearRunsForJob(api: ApiHelper, jobId: string) {
  await api.deleteRunsForJob(jobId);
}
