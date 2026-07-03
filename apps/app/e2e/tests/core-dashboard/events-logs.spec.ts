import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let eventKey: string;
let workflowRunId: string;

test.describe("Events and logs dashboard", () => {
  test.describe.configure({ timeout: 120_000 });
  test.setTimeout(120_000);

  test.beforeEach(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    eventKey = data.name("approval-event");
    const workflow = await api.createWorkflow({
      name: data.name("event-workflow"),
      steps: [
        {
          step_ref: "await-approval",
          step_type: "wait_for_event",
          event_key: eventKey,
          event_timeout_secs: 300,
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));
    data.cleanup.add(() =>
      api.sendEvent(eventKey, { approved: true }).catch(() => undefined)
    );

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      case_id: eventKey,
    });
    workflowRunId = workflowRun.id;

    await api.waitForEventTrigger(
      (event) =>
        event.event_key === eventKey &&
        event.workflow_run_id === workflowRunId &&
        event.status === "waiting",
      60_000
    );
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("filters the events timeline by backend event status", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      "/app/events?status=waiting",
      page.getByRole("button", { name: "Waiting" })
    );
    await expect(page.getByText(eventKey)).toBeVisible();
    await expect(
      page.getByRole("row", {
        name: new RegExp(`Waiting ${escapeRegExp(eventKey)}`),
      })
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "workflow_step" })
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "event", exact: true })
    ).toBeVisible();

    await api.sendEvent(eventKey, { approved: true });
    await api.waitForEventTrigger(
      (event) => event.event_key === eventKey && event.status === "received",
      60_000
    );

    await gotoAndExpect(
      page,
      "/app/events?status=received",
      page.getByRole("button", { name: "Received" })
    );
    await expect(page.getByText(eventKey)).toBeVisible();
    await expect(page.getByText("Received").first()).toBeVisible();

    await gotoAndExpect(
      page,
      "/app/events?status=waiting",
      page.getByRole("button", { name: "Waiting" })
    );
    await expect(page.getByText(eventKey)).toHaveCount(0);
  });

  test("searches log entries with event context", async ({ page }) => {
    await api.sendEvent(eventKey, { approved: true });
    await api.waitForEventTrigger(
      (event) => event.event_key === eventKey && event.status === "received",
      60_000
    );

    await gotoAndExpect(
      page,
      "/app/logs",
      page.getByRole("region", { name: "Logs" })
    );

    const logsRegion = page.getByRole("region", { name: "Logs" });
    await page.getByLabel("Search").fill(eventKey);
    await expect(logsRegion.getByText(eventKey)).toBeVisible();
    await expect(
      logsRegion.getByRole("row", {
        name: new RegExp(`Received ${escapeRegExp(eventKey)}`),
      })
    ).toBeVisible();
    await expect(
      logsRegion.getByRole("cell", { name: "workflow_step" })
    ).toBeVisible();
    await expect(
      logsRegion.getByRole("cell", { name: "event", exact: true })
    ).toBeVisible();
  });
});

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
