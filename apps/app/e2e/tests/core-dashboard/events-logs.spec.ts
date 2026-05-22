import type { Locator, Page } from "@playwright/test";
import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let eventKey: string;
let workflowRunId: string;
const routeLoad = { timeout: 90_000, waitUntil: "domcontentloaded" } as const;

test.describe("Events and logs dashboard", () => {
  test.describe.configure({ mode: "serial", timeout: 120_000 });
  test.setTimeout(120_000);

  test.beforeAll(async () => {
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

  test.afterAll(async () => {
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
    await expect(page.getByText("event | workflow_step")).toBeVisible();

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
      page.getByRole("table", { name: "Logs" })
    );

    await page.getByLabel("Search").fill(eventKey);
    await expect(page.getByText(eventKey)).toBeVisible();
    await expect(
      page.getByRole("row", { name: new RegExp(`received ${eventKey}`) })
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "workflow_step" })
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "event", exact: true })
    ).toBeVisible();
  });
});

async function gotoAndExpect(page: Page, path: string, locator: Locator) {
  for (let attempt = 1; attempt <= 2; attempt += 1) {
    await page.goto(path, routeLoad);
    try {
      await expect(locator).toBeVisible();
      return;
    } catch (error) {
      if (attempt === 2) {
        throw error;
      }
    }
  }
}
