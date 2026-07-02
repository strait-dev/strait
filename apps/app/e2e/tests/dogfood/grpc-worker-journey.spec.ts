import { type ChildProcess, spawn } from "node:child_process";
import fs from "node:fs";
import { resolve } from "node:path";
import type { ApiHelper } from "../../fixtures";
import { expect, test } from "../../fixtures";

test.describe("Dogfood gRPC worker journey", () => {
  test.describe.configure({ timeout: 120_000 });

  test("dispatches a worker-mode job through a real local gRPC worker", async ({
    api,
    page,
  }) => {
    const suffix = Date.now();
    const workerId = `dogfood-worker-${suffix}`;
    const queueName = `dogfood-${suffix}`;
    const jobName = `e2e-dogfood-worker-job-${suffix}`;
    const apiKey = await api.createApiKey({
      expires_in_days: 30,
      name: `e2e dogfood worker ${suffix}`,
      scopes: ["workers:connect"],
    });
    const worker = startDogfoodWorker(apiKey.key, workerId, queueName);

    try {
      await waitForWorker(api, workerId);

      await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
      await openCreateDialog(page, "Create job", "Create job");
      await page.getByLabel("Name", { exact: true }).fill(jobName);
      await page.getByRole("combobox", { name: "Execution mode" }).click();
      await page.getByRole("option", { name: "gRPC worker" }).click();
      await page.getByLabel("Queue name").fill(queueName);
      await page.getByLabel("Timeout seconds").fill("10");
      await page.getByRole("button", { name: "Create job" }).click();

      await page.getByLabel("Search").fill(jobName);
      await expect(page.getByText(jobName).first()).toBeVisible({
        timeout: 15_000,
      });
      const jobId = await waitForJobIdByName(api, jobName);

      await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("heading", { name: jobName })).toBeVisible();

      const runId = await triggerJobThroughUI(page, api, jobId);
      await api.waitForRunStatus(runId, ["completed"], 60_000);

      await page.goto(`/app/runs/${runId}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("heading", { name: runId })).toBeVisible();
      await expect(page.getByText("completed").first()).toBeVisible();
    } finally {
      stopWorker(worker);
      await api.revokeApiKey(apiKey.id).catch(() => undefined);
      const jobs = await api.listJobs({ search: jobName, limit: 100 });
      await Promise.all(
        jobs.data
          .filter((job) => job.name === jobName)
          .map((job) => api.deleteJob(job.id).catch(() => undefined))
      );
    }
  });
});

async function openCreateDialog(
  page: import("@playwright/test").Page,
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
      `Dogfood worker binary was not found at ${workerBin}. Run the suite through \`bun run dogfood -- grpc\` or \`bun run dogfood -- jobs\`.`
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

async function triggerJobThroughUI(
  page: import("@playwright/test").Page,
  api: ApiHelper,
  jobId: string
) {
  const button = page.getByRole("button", { name: "Trigger" });
  await expect(button).toBeVisible({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt++) {
    await button.click();
    const runId = await waitForLatestJobRunId(api, jobId, 8000).catch(
      () => undefined
    );
    if (runId) {
      return runId;
    }
  }
  return await waitForLatestJobRunId(api, jobId, 20_000);
}

async function waitForLatestJobRunId(
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
