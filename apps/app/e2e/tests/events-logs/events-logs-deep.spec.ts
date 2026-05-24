import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Events and logs deep coverage", () => {
  test.describe.configure({ timeout: 180_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("persists waiting and received event payload context across API and UI", async ({
    page,
  }) => {
    const eventKey = data.name("deep-event");
    const responsePayload = { approved: true, reviewer: "apps-app-e2e" };
    const workflow = await api.createWorkflow({
      name: data.name("deep-event-workflow"),
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

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      case_id: eventKey,
      expected: "approval",
    });
    data.cleanup.add(() =>
      api.sendEvent(eventKey, responsePayload).catch(() => undefined)
    );
    const waitingEvent = await api.waitForEventTrigger(
      (event) =>
        event.event_key === eventKey &&
        event.workflow_run_id === workflowRun.id &&
        event.status === "waiting",
      60_000
    );
    expect(waitingEvent.source_type).toBe("workflow_step");

    await api.sendEvent(eventKey, responsePayload);
    const receivedEvent = await api.waitForEventTrigger(
      (event) =>
        event.event_key === eventKey &&
        event.workflow_run_id === workflowRun.id &&
        event.status === "received",
      60_000
    );
    expect(receivedEvent.response_payload).toMatchObject(responsePayload);

    const fetchedEvent = await api.getEvent(eventKey);
    expect(fetchedEvent.id).toBe(receivedEvent.id);
    expect(fetchedEvent.status).toBe("received");
    expect(fetchedEvent.workflow_run_id).toBe(workflowRun.id);

    await gotoAndExpect(
      page,
      "/app/events?status=received",
      page.getByRole("button", { name: "Received" })
    );
    await expect(page.getByText(eventKey)).toBeVisible();
    await expect(page.getByText("event | workflow_step")).toBeVisible();
  });

  test("searches log rows and exposes source/type context for operators", async ({
    page,
  }) => {
    const eventKey = data.name("expanded-log");
    const workflow = await api.createWorkflow({
      name: data.name("expanded-log-workflow"),
      steps: [
        {
          step_ref: "await-log-event",
          step_type: "wait_for_event",
          event_key: eventKey,
          event_timeout_secs: 300,
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      trace_id: eventKey,
      action: "inspect",
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
      "/app/logs",
      page.getByRole("table", { name: "Logs" })
    );
    await page.getByLabel("Search").fill(eventKey);

    const row = page.getByRole("row", { name: new RegExp(eventKey) });
    await expect(row).toBeVisible();
    await expect(row).toContainText("workflow_step");
    await expect(row).toContainText("event");
  });
});
