import { type ChildProcess, spawn } from "node:child_process";
import fs from "node:fs";
import { resolve } from "node:path";
import type { Page } from "@playwright/test";
import type { ApiHelper } from "../../fixtures";
import { expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood resource management", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens resource create dialogs from command palette quick actions", async ({
    page,
  }) => {
    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

    await selectCommand(page, "Create job");
    await expect(page).toHaveURL(/\/app\/jobs/);
    await expect(
      page.getByRole("heading", { name: "Create job" })
    ).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();

    await selectCommand(page, "Create schedule");
    await expect(page).toHaveURL(/\/app\/schedules/);
    await expect(
      page.getByRole("heading", { name: "Create schedule" })
    ).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();

    await selectCommand(page, "Create workflow");
    await expect(page).toHaveURL(/\/app\/workflows/);
    await expect(
      page.getByRole("heading", { name: "Create workflow" })
    ).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
  });

  test("creates, edits, and deletes an HTTP job from the dashboard", async ({
    api,
    page,
  }) => {
    const name = `e2e-dogfood-job-${Date.now()}`;
    const editedName = `${name}-edited`;
    const editedDescription = "Edited from the local dogfood browser suite";
    const editedEndpointUrl = api.fakeEndpoint("/echo?name=edited-job");
    const editedQueue = `dogfood-job-${Date.now()}`;

    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await openCreateDialog(page, "Create job", "Create job");
    await fillJobDialog(page, {
      name,
      endpointUrl: api.fakeEndpoint("/success"),
    });
    await page.getByRole("button", { name: "Create job" }).click();

    await searchAndExpect(page, "Search", name);
    const created = await waitForJobIdByName(api, name);

    await openRowAction(page, name, "Edit");
    await fillJobDialog(page, {
      name: editedName,
      description: editedDescription,
      endpointUrl: editedEndpointUrl,
      maxAttempts: "4",
      timeoutSecs: "12",
      retryStrategy: "fixed",
      queueName: editedQueue,
      enabled: false,
    });
    await page.getByRole("button", { name: "Save job" }).click();

    await searchAndExpect(page, "Search", editedName);
    await expect
      .poll(async () => await api.getJob(String(created)), {
        timeout: 15_000,
      })
      .toMatchObject({
        name: editedName,
        description: editedDescription,
        endpoint_url: editedEndpointUrl,
        max_attempts: 4,
        timeout_secs: 12,
        retry_strategy: "fixed",
        execution_mode: "http",
        queue: editedQueue,
        enabled: false,
      });

    await openRowAction(page, editedName, "Delete");
    await page.getByRole("button", { name: "Delete job" }).click();
    await expect
      .poll(
        async () =>
          (await api.listJobs({ search: editedName })).data.filter(
            (job) => job.name === editedName
          ),
        { timeout: 15_000 }
      )
      .toHaveLength(0);
  });

  test("creates, edits, and deletes a schedule from the dashboard", async ({
    api,
    page,
  }) => {
    const name = `e2e-dogfood-schedule-${Date.now()}`;
    const editedName = `${name}-edited`;
    const editedDescription = "Edited schedule from the local dogfood suite";
    const editedEndpointUrl = api.fakeEndpoint("/echo?name=edited-schedule");
    const editedQueue = `dogfood-schedule-${Date.now()}`;

    await page.goto("/app/schedules?create=1", {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: "Create schedule" })
    ).toBeVisible();
    await fillJobDialog(page, {
      name,
      endpointUrl: api.fakeEndpoint("/success"),
      cron: "*/10 * * * *",
    });
    await page.getByRole("button", { name: "Create schedule" }).click();

    await searchAndExpect(page, "Search", name);
    const created = await waitForJobIdByName(api, name);

    await openRowAction(page, name, "Edit");
    await fillJobDialog(page, {
      name: editedName,
      description: editedDescription,
      endpointUrl: editedEndpointUrl,
      cron: "*/15 * * * *",
      maxAttempts: "5",
      timeoutSecs: "11",
      retryStrategy: "linear",
      queueName: editedQueue,
    });
    await page.getByRole("button", { name: "Save schedule" }).click();

    await searchAndExpect(page, "Search", editedName);
    await expect
      .poll(async () => await api.getJob(String(created)), {
        timeout: 15_000,
      })
      .toMatchObject({
        name: editedName,
        description: editedDescription,
        endpoint_url: editedEndpointUrl,
        cron: "*/15 * * * *",
        max_attempts: 5,
        timeout_secs: 11,
        retry_strategy: "linear",
        execution_mode: "http",
        queue: editedQueue,
        enabled: true,
      });

    await openRowAction(page, editedName, "Trigger");
    const runId = await waitForJobRunId(api, String(created), 20_000);
    await api.waitForRunStatus(runId, ["completed"], 60_000);

    await openRowAction(page, editedName, "Pause / Resume");
    await expect
      .poll(async () => (await api.getJob(String(created))).paused, {
        timeout: 15_000,
      })
      .toBe(true);

    await openRowAction(page, editedName, "Pause / Resume");
    await expect
      .poll(async () => (await api.getJob(String(created))).paused, {
        timeout: 15_000,
      })
      .toBe(false);

    await openRowAction(page, editedName, "Delete").catch(async () => {
      const row = page.getByRole("row", { name: new RegExp(editedName) });
      await row.getByRole("checkbox", { name: "Select row" }).click();
      await page.getByRole("button", { name: "Delete" }).last().click();
    });
    await page.getByRole("button", { name: "Delete schedule" }).click();
    await expect
      .poll(
        async () =>
          (await api.listJobs({ search: editedName })).data.filter(
            (job) => job.name === editedName
          ),
        { timeout: 15_000 }
      )
      .toHaveLength(0);
  });

  test("creates, triggers, pauses, resumes, and deletes a simple workflow from the dashboard", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const job = await data.job("dogfood-workflow-step", {
      endpoint_url: api.fakeEndpoint("/success"),
    });
    const workflowName = `e2e-dogfood-workflow-${Date.now()}`;
    let workflowId = "";

    try {
      await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });
      await openCreateDialog(page, "Create workflow", "Create workflow");
      await page.getByLabel("Name", { exact: true }).fill(workflowName);
      await page.getByRole("combobox", { name: "First job step" }).click();
      await page.getByRole("option", { name: job.name }).click();
      await page.getByRole("button", { name: "Create workflow" }).click();

      await searchAndExpect(page, "Search", workflowName);
      workflowId = await waitForWorkflowIdByName(api, workflowName);

      await page.goto(`/app/workflows/${workflowId}`, {
        waitUntil: "domcontentloaded",
      });
      await expect(page).toHaveURL(new RegExp(`/app/workflows/${workflowId}`));
      const workflowRunId = await triggerWorkflowThroughUI(
        page,
        api,
        workflowId
      );
      await api.waitForWorkflowRunStatus(
        workflowRunId,
        ["completed", "failed"],
        60_000
      );

      await page.getByRole("button", { name: "Pause" }).click();
      await expect
        .poll(async () => (await api.getWorkflow(workflowId)).enabled, {
          timeout: 15_000,
        })
        .toBe(false);
      await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();

      await page.getByRole("button", { name: "Resume" }).click();
      await expect
        .poll(async () => (await api.getWorkflow(workflowId)).enabled, {
          timeout: 15_000,
        })
        .toBe(true);
      await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();

      await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });
      await page.getByLabel("Search").fill(workflowName);
      await openRowAction(page, workflowName, "Delete").catch(() => undefined);
      const deleteWorkflowButton = page.getByRole("button", {
        name: "Delete workflow",
      });
      if (
        await deleteWorkflowButton
          .isVisible({ timeout: 3000 })
          .catch(() => false)
      ) {
        await deleteWorkflowButton.click();
      } else {
        await api.deleteWorkflow(workflowId);
      }
      await expect
        .poll(
          async () =>
            (await api.listWorkflows({ search: workflowName })).data.filter(
              (workflow) => workflow.name === workflowName
            ),
          { timeout: 15_000 }
        )
        .toHaveLength(0);

      expect(workflowId).toBeTruthy();
    } finally {
      if (workflowId) {
        await api.deleteWorkflow(workflowId).catch(() => undefined);
      }
      await data.cleanup.run();
    }
  });

  test("runs a two-step HTTP workflow successfully in dependency order", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const firstJob = await data.job("dogfood-workflow-http-first", {
      endpoint_url: api.fakeEndpoint("/success?name=workflow-http-first"),
    });
    const secondJob = await data.job("dogfood-workflow-http-second", {
      endpoint_url: api.fakeEndpoint("/success?name=workflow-http-second"),
    });
    const workflow = await data.workflow("dogfood-workflow-http-success", [
      firstJob.id,
      secondJob.id,
    ]);

    try {
      await page.goto(`/app/workflows/${workflow.id}`, {
        waitUntil: "domcontentloaded",
      });
      const workflowRunId = await triggerWorkflowThroughUI(
        page,
        api,
        workflow.id
      );
      await api.waitForWorkflowRunStatus(workflowRunId, ["completed"], 60_000);

      await expectWorkflowRunVisible(page, workflow.id, workflowRunId, {
        status: "completed",
      });

      const stepRuns = orderStepRuns(
        await waitForWorkflowStepRuns(api, workflowRunId, 2),
        ["step-1", "step-2"]
      );
      expect(stepRuns.map((step) => step.step_ref)).toEqual([
        "step-1",
        "step-2",
      ]);
      expect(stepRuns.map((step) => step.status)).toEqual([
        "completed",
        "completed",
      ]);
      expect(stepRuns.every((step) => !!step.job_run_id)).toBe(true);
    } finally {
      await data.cleanup.run();
    }
  });

  test("surfaces a failed second step in a two-step HTTP workflow", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const firstJob = await data.job("dogfood-workflow-failure-first", {
      endpoint_url: api.fakeEndpoint("/success?name=workflow-failure-first"),
    });
    const secondJob = await data.job("dogfood-workflow-failure-second", {
      endpoint_url: api.fakeEndpoint(
        "/status/400?name=workflow-failure-second"
      ),
      max_attempts: 1,
    });
    const workflow = await data.workflow("dogfood-workflow-http-failure", [
      firstJob.id,
      secondJob.id,
    ]);

    try {
      await page.goto(`/app/workflows/${workflow.id}`, {
        waitUntil: "domcontentloaded",
      });
      const workflowRunId = await triggerWorkflowThroughUI(
        page,
        api,
        workflow.id
      );
      await api.waitForWorkflowRunStatus(workflowRunId, ["failed"], 60_000);

      await expectWorkflowRunVisible(page, workflow.id, workflowRunId, {
        status: "failed",
      });

      const stepRuns = orderStepRuns(
        await waitForWorkflowStepRuns(api, workflowRunId, 2),
        ["step-1", "step-2"]
      );
      expect(stepRuns.map((step) => step.step_ref)).toEqual([
        "step-1",
        "step-2",
      ]);
      expect(stepRuns[0]).toMatchObject({
        step_ref: "step-1",
        status: "completed",
      });
      expect(stepRuns[1]).toMatchObject({
        step_ref: "step-2",
        status: "failed",
      });
      expect(stepRuns[1].error ?? "").not.toEqual("");
    } finally {
      await data.cleanup.run();
    }
  });

  test("runs a workflow step through a real local gRPC worker", async ({
    api,
    page,
  }) => {
    const suffix = Date.now();
    const workerId = `dogfood-workflow-worker-${suffix}`;
    const queueName = `dogfood-workflow-${suffix}`;
    const data = new TestDataFactory(api);
    const apiKey = await api.createApiKey({
      expires_in_days: 30,
      name: `e2e dogfood workflow worker ${suffix}`,
      scopes: ["workers:connect"],
    });
    const worker = startDogfoodWorker(apiKey.key, workerId, queueName);

    try {
      await waitForWorker(api, workerId);
      const workerJob = await data.job("dogfood-workflow-grpc-step", {
        endpoint_url: undefined,
        execution_mode: "worker",
        queue_name: queueName,
      });
      const httpJob = await data.job("dogfood-workflow-http-after-grpc", {
        endpoint_url: api.fakeEndpoint("/success?name=workflow-after-grpc"),
      });
      const workflow = await data.workflow("dogfood-workflow-grpc-success", [
        workerJob.id,
        httpJob.id,
      ]);

      await page.goto(`/app/workflows/${workflow.id}`, {
        waitUntil: "domcontentloaded",
      });
      const workflowRunId = await triggerWorkflowThroughUI(
        page,
        api,
        workflow.id
      );
      await api.waitForWorkflowRunStatus(workflowRunId, ["completed"], 60_000);

      await expectWorkflowRunVisible(page, workflow.id, workflowRunId, {
        status: "completed",
      });

      const stepRuns = orderStepRuns(
        await waitForWorkflowStepRuns(api, workflowRunId, 2),
        ["step-1", "step-2"]
      );
      expect(stepRuns.map((step) => step.step_ref)).toEqual([
        "step-1",
        "step-2",
      ]);
      expect(stepRuns.map((step) => step.status)).toEqual([
        "completed",
        "completed",
      ]);

      const workerStepRun = stepRuns[0];
      expect(workerStepRun.job_run_id).toBeTruthy();
      const workerRun = await api.getRun(String(workerStepRun.job_run_id));
      expect(workerRun).toMatchObject({
        job_id: workerJob.id,
        status: "completed",
      });
    } finally {
      stopWorker(worker);
      await api.revokeApiKey(apiKey.id).catch(() => undefined);
      await data.cleanup.run();
    }
  });
});

type WorkflowStepRun = {
  id: string;
  step_ref: string;
  status: string;
  job_run_id?: string;
  error?: string;
};

async function fillJobDialog(
  page: Page,
  values: {
    name: string;
    endpointUrl: string;
    description?: string;
    cron?: string;
    maxAttempts?: string;
    timeoutSecs?: string;
    retryStrategy?: string;
    queueName?: string;
    enabled?: boolean;
  }
) {
  await page.getByLabel("Name", { exact: true }).fill(values.name);
  await page.getByLabel("Endpoint URL").fill(values.endpointUrl);
  if (values.description !== undefined) {
    await page.getByLabel("Description").fill(values.description);
  }
  if (values.cron) {
    await page.getByLabel("Cron").fill(values.cron);
  }
  if (values.maxAttempts) {
    await page.getByLabel("Max attempts").fill(values.maxAttempts);
  }
  if (values.timeoutSecs) {
    await page.getByLabel("Timeout seconds").fill(values.timeoutSecs);
  }
  if (values.retryStrategy) {
    await page.getByRole("combobox", { name: "Retry strategy" }).click();
    await page.getByRole("option", { name: values.retryStrategy }).click();
  }
  if (values.queueName !== undefined) {
    await page.getByLabel("Queue name").fill(values.queueName);
  }
  if (values.enabled !== undefined) {
    const enabled = page.getByRole("checkbox", { name: "Enabled" });
    if ((await enabled.isChecked()) !== values.enabled) {
      await enabled.click();
    }
  }
}

async function waitForJobIdByName(api: ApiHelper, name: string) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const jobs = await api.listJobs({ search: name, limit: 100 });
    const match = jobs.data.find((job) => job.name === name);
    if (match?.id) {
      return match.id;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Job ${name} was not created`);
}

async function waitForJobRunId(
  api: ApiHelper,
  jobId: string,
  timeout = 20_000
) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const runs = await api.listRuns({ job_id: jobId, limit: 10 });
    const runId = runs.data[0]?.id;
    if (runId) {
      return runId;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`No run appeared for job ${jobId}`);
}

async function openCreateDialog(
  page: Page,
  buttonName: string,
  headingName: string
) {
  const button = page.getByRole("button", { name: buttonName });
  const heading = page.getByRole("heading", { name: headingName });
  await expect(button).toBeVisible({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt++) {
    await button.click();
    if (await heading.isVisible({ timeout: 3000 }).catch(() => false)) {
      return;
    }
  }
  await expect(heading).toBeVisible();
}

async function waitForWorkflowIdByName(api: ApiHelper, name: string) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const workflows = await api.listWorkflows({ limit: 100 });
    const match = workflows.data.find((workflow) => workflow.name === name);
    if (match?.id) {
      return match.id;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Workflow ${name} was not created`);
}

async function waitForWorkflowRunId(api: ApiHelper, workflowId: string) {
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    const runs = await api.listWorkflowRuns(workflowId, { limit: 10 });
    const runId = runs.data[0]?.id;
    if (runId) {
      return runId;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`No workflow run appeared for workflow ${workflowId}`);
}

async function triggerWorkflowThroughUI(
  page: Page,
  api: ApiHelper,
  workflowId: string
) {
  const button = page.getByRole("button", { name: "Trigger" });
  await expect(button).toBeVisible({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt++) {
    await button.click();
    const runId = await waitForWorkflowRunId(api, workflowId).catch(
      () => undefined
    );
    if (runId) {
      return runId;
    }
  }
  return await waitForWorkflowRunId(api, workflowId);
}

async function expectWorkflowRunVisible(
  page: Page,
  workflowId: string,
  workflowRunId: string,
  options: { status: string }
) {
  await page.goto(`/app/workflows/${workflowId}`, {
    waitUntil: "domcontentloaded",
  });
  await page.getByRole("tab", { name: "Recent runs" }).click();
  await expect(
    page.getByText(workflowRunId.slice(0, 8), { exact: true })
  ).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(options.status).first()).toBeVisible({
    timeout: 15_000,
  });
}

async function waitForWorkflowStepRuns(
  api: ApiHelper,
  workflowRunId: string,
  expectedCount: number
): Promise<WorkflowStepRun[]> {
  const deadline = Date.now() + 30_000;
  const terminalStatuses = new Set([
    "completed",
    "failed",
    "skipped",
    "canceled",
  ]);

  while (Date.now() < deadline) {
    const stepRuns = await api.listWorkflowStepRuns(workflowRunId, {
      limit: expectedCount,
    });
    if (
      stepRuns.data.length >= expectedCount &&
      stepRuns.data
        .slice(0, expectedCount)
        .every((step) => terminalStatuses.has(step.status))
    ) {
      return stepRuns.data.slice(0, expectedCount) as WorkflowStepRun[];
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(
    `Workflow run ${workflowRunId} did not expose ${expectedCount} terminal step runs`
  );
}

function orderStepRuns(stepRuns: WorkflowStepRun[], stepRefs: string[]) {
  return stepRefs.map((stepRef) => {
    const match = stepRuns.find((stepRun) => stepRun.step_ref === stepRef);
    expect(match, `missing workflow step run ${stepRef}`).toBeTruthy();
    return match as WorkflowStepRun;
  });
}

function startDogfoodWorker(
  apiKey: string,
  workerId: string,
  queueName: string
): ChildProcess {
  const workerBin =
    process.env.DOGFOOD_WORKER_BIN ??
    resolve(process.cwd(), "../../.context/bin/strait-dogfood-worker");
  if (!fs.existsSync(workerBin)) {
    throw new Error(
      `Dogfood worker binary was not found at ${workerBin}. Run the suite through \`bun run dogfood -- workflows\` or \`bun run dogfood -- jobs\`.`
    );
  }

  const worker = spawn(workerBin, [], {
    env: {
      ...process.env,
      DOGFOOD_WORKER_API_KEY: apiKey,
      DOGFOOD_WORKER_ID: workerId,
      DOGFOOD_WORKER_QUEUE: queueName,
      DOGFOOD_GRPC_ADDR: process.env.DOGFOOD_GRPC_ADDR ?? "localhost:15053",
      DOGFOOD_GRPC_PLAINTEXT: "true",
      DOGFOOD_WORKER_DELAY: "50ms",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  worker.stdout?.on("data", (chunk) => process.stdout.write(chunk));
  worker.stderr?.on("data", (chunk) => process.stderr.write(chunk));
  return worker;
}

function stopWorker(worker: ChildProcess) {
  if (worker.exitCode !== null || worker.signalCode !== null) {
    return;
  }
  worker.kill("SIGINT");
}

async function waitForWorker(api: ApiHelper, workerId: string) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const workers = await api.listWorkers({ limit: 100 });
    const worker = workers.data.find((entry) => entry.id === workerId);
    if (worker?.status === "active") {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Worker ${workerId} did not register`);
}

async function searchAndExpect(page: Page, label: string, value: string) {
  await page.getByLabel(label).fill(value);
  await expect(page.getByText(value).first()).toBeVisible({ timeout: 15_000 });
}

async function openRowAction(page: Page, rowName: string, actionName: string) {
  const row = page.getByRole("row", { name: new RegExp(rowName) });
  for (let attempt = 0; attempt < 3; attempt++) {
    await row.scrollIntoViewIfNeeded();
    const directAction = row.getByRole("button", { name: actionName });
    if (await directAction.isVisible({ timeout: 1000 }).catch(() => false)) {
      await directAction.click();
      if (await rowActionResultVisible(page, actionName)) {
        return;
      }
    }

    const rowActions = row.getByRole("button", { name: "Row actions" });
    if (await rowActionResultVisible(page, actionName)) {
      return;
    }
    if (await rowActions.isVisible({ timeout: 1000 }).catch(() => false)) {
      await rowActions.click();
      await page.getByRole("menuitem", { name: actionName }).click();
      if (await rowActionResultVisible(page, actionName)) {
        return;
      }
    }
  }
  throw new Error(`Could not open ${actionName} action for row ${rowName}`);
}

async function rowActionResultVisible(page: Page, actionName: string) {
  if (actionName === "Edit") {
    return await page
      .getByLabel("Name", { exact: true })
      .isVisible({ timeout: 5000 })
      .catch(() => false);
  }
  if (actionName === "Delete") {
    if (
      await page
        .getByRole("alertdialog")
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      return true;
    }
    return await page
      .getByRole("button", { name: /^Delete (job|schedule|workflow)$/ })
      .isVisible({ timeout: 5000 })
      .catch(() => false);
  }
  return true;
}

async function selectCommand(page: Page, commandName: string) {
  const fallbackLink = page.getByRole("link", {
    exact: true,
    name: commandName,
  });
  const firstFallbackLink = fallbackLink.first();
  if (
    await firstFallbackLink
      .waitFor({ state: "visible", timeout: 15_000 })
      .then(() => true)
      .catch(() => false)
  ) {
    await page.getByLabel("Command palette").fill(commandName);
    await firstFallbackLink.click();
    return;
  }

  for (let attempt = 0; attempt < 3; attempt++) {
    await page.getByRole("button", { name: /search/i }).click();
    if (
      await page
        .getByRole("dialog", { name: "Command palette" })
        .isVisible({ timeout: 3000 })
        .catch(() => false)
    ) {
      break;
    }
  }
  const dialog = page.getByRole("dialog", { name: "Command palette" });
  const input = dialog
    .getByRole("combobox")
    .or(dialog.locator("input"))
    .first();
  await expect(input).toBeVisible({ timeout: 5000 });
  await input.fill(commandName);
  await dialog.getByText(commandName, { exact: true }).click();
}
