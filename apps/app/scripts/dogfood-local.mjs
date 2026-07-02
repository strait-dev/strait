import { spawn } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import pg from "pg";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const appDir = resolve(__dirname, "..");
const repoRoot = resolve(appDir, "../..");
const straitDir = resolve(repoRoot, "apps/strait");
const binaryPath = resolve(repoRoot, ".context/bin/strait-dogfood");
const workerBinaryPath = resolve(
  repoRoot,
  ".context/bin/strait-dogfood-worker"
);
const dogfoodStateDir = resolve(repoRoot, ".context/dogfood");
const dogfoodLockPath = resolve(dogfoodStateDir, "dogfood-local.lock");
const nativePostgresDataDir = resolve(dogfoodStateDir, "postgres");
const nativePostgresSocketDir = resolve(dogfoodStateDir, "postgres-socket");
const nativeRedisDataDir = resolve(dogfoodStateDir, "redis");
const devVarsPath = resolve(appDir, ".dev.vars");

const apiPort =
  process.env.DOGFOOD_STRAIT_PORT || process.env.E2E_STRAIT_PORT || "18082";
const grpcPort =
  process.env.DOGFOOD_STRAIT_GRPC_PORT ||
  process.env.E2E_STRAIT_GRPC_PORT ||
  "15053";
const databaseUrl =
  process.env.DATABASE_URL ||
  "postgres://strait:strait@localhost:15432/strait?sslmode=disable";
const redisUrl = process.env.REDIS_URL || "redis://localhost:16379";
const postgresEndpoint = serviceEndpoint(databaseUrl, "localhost", "15432");
const redisEndpoint = serviceEndpoint(redisUrl, "localhost", "16379");
const straitApiUrl =
  process.env.STRAIT_API_URL || `http://localhost:${apiPort}`;
const appUrl = process.env.EXPECT_BASE_URL || "http://localhost:5173";
const sequinBaseUrl = process.env.SEQUIN_BASE_URL || "http://localhost:7376";
const localEncryptionKey =
  process.env.ENCRYPTION_KEY ||
  process.env.SECRET_ENCRYPTION_KEY ||
  "0123456789abcdef0123456789abcdef";
const sequinEndpoint = serviceEndpoint(sequinBaseUrl, "localhost", "7376");
const sequinReceivePathPattern = /^\/api\/http_pull_consumers\/[^/]+\/receive$/;
const sequinAckPathPattern =
  /^\/api\/http_pull_consumers\/[^/]+\/(?:ack|nack)$/;
const { Client } = pg;

const batchMap = {
  smoke: ["tests/dogfood/smoke.spec.ts"],
  harness: ["tests/harness"],
  dashboard: [
    "tests/dashboard",
    "tests/core-dashboard/dashboard-metrics.spec.ts",
  ],
  jobs: [
    "tests/dogfood/resource-management.spec.ts",
    "tests/dogfood/http-job-journey.spec.ts",
    "tests/dogfood/grpc-worker-journey.spec.ts",
    "tests/jobs",
    "tests/crud/job-lifecycle.spec.ts",
  ],
  grpc: ["tests/dogfood/grpc-worker-journey.spec.ts"],
  runs: [
    "tests/dogfood/runs-journey.spec.ts",
    "tests/runs",
    "tests/core-dashboard/jobs-runs-lifecycle.spec.ts",
  ],
  schedules: ["tests/dogfood/resource-management.spec.ts", "tests/schedules"],
  workflows: ["tests/dogfood/resource-management.spec.ts", "tests/workflows"],
  operations: ["tests/dogfood/operations-journey.spec.ts"],
  navigation: ["tests/dogfood/navigation-journey.spec.ts"],
  webhooks: [
    "tests/dogfood/operations-journey.spec.ts",
    "tests/webhooks",
    "tests/core-dashboard/webhook-deliveries.spec.ts",
  ],
  dlq: ["tests/dogfood/operations-journey.spec.ts", "tests/dlq"],
  events: ["tests/dogfood/operations-journey.spec.ts", "tests/events-logs"],
  settings: [
    "tests/dogfood/settings-projects-journey.spec.ts",
    "tests/settings",
    "tests/org",
    "tests/projects",
  ],
  permissions: [
    "tests/dogfood/permissions-projects.spec.ts",
    "tests/settings/rbac-permissions.spec.ts",
    "tests/core-dashboard/project-isolation.spec.ts",
  ],
  visual: [
    "tests/dashboard/charts.spec.ts",
    "tests/interaction/responsive.spec.ts",
    "tests/interaction/theme.spec.ts",
    "tests/interaction/a11y-keyboard.spec.ts",
  ],
};

batchMap.all = [
  ...batchMap.smoke,
  ...batchMap.dashboard,
  ...batchMap.jobs,
  ...batchMap.runs,
  ...batchMap.schedules,
  ...batchMap.workflows,
  ...batchMap.operations,
  ...batchMap.navigation,
  ...batchMap.webhooks,
  ...batchMap.dlq,
  ...batchMap.events,
  ...batchMap.settings,
  ...batchMap.permissions,
].filter((target, index, targets) => targets.indexOf(target) === index);

const cleanupTasks = [];
const options = parseArgs(process.argv.slice(2));
const playwrightArgs = resolvePlaywrightArgs(options.targets);

main().catch(async (error) => {
  await cleanup();
  console.error(error instanceof Error ? error.stack || error.message : error);
  process.exit(1);
});

async function main() {
  for (const signal of ["SIGINT", "SIGTERM"]) {
    process.once(signal, async () => {
      await cleanup();
      process.kill(process.pid, signal);
    });
  }

  if (options.list) {
    printBatches();
    return;
  }

  if (options.doctor) {
    await runDoctor();
    return;
  }

  acquireDogfoodRunLock();

  if (options.reset) {
    console.log("Resetting local dogfood state...");
    await resetLocalState();
  }

  console.log("Checking local Postgres and Redis...");
  await ensureLocalDependencies();
  console.log("Exporting local app environment...");
  await exportInfisicalEnv();
  console.log("Starting or reusing local Sequin...");
  await ensureLocalSequin();
  console.log("Building Strait dogfood binary...");
  await buildStraitBinary();
  console.log("Building Strait dogfood gRPC worker...");
  await buildDogfoodWorkerBinary();
  console.log("Starting or reusing Strait backend...");
  await startBackend();
  await cleanupDogfoodDatabase();
  printRuntimeSummary();

  await run("bun", ["run", "e2e", "--", ...playwrightArgs], {
    cwd: appDir,
    env: {
      ...process.env,
      ...readDotEnv(devVarsPath),
      DATABASE_URL: databaseUrl,
      AUTH_DATABASE_URL: process.env.AUTH_DATABASE_URL || databaseUrl,
      ENCRYPTION_KEY: localEncryptionKey,
      REDIS_URL: redisUrl,
      STRAIT_API_URL: straitApiUrl,
      EXPECT_BASE_URL: appUrl,
      DOGFOOD_WORKER_BIN: workerBinaryPath,
      DOGFOOD_GRPC_ADDR: `localhost:${grpcPort}`,
      DOGFOOD_LOCAL_ENV: "1",
      E2E_REUSE_POSTGRES: "1",
    },
  });

  await cleanup();
}

function acquireDogfoodRunLock() {
  fs.mkdirSync(dogfoodStateDir, { recursive: true });

  try {
    return createDogfoodRunLock();
  } catch (error) {
    if (error?.code !== "EEXIST") {
      throw error;
    }
  }

  const existing = readDogfoodRunLock();
  if (existing?.pid && !processIsRunning(existing.pid)) {
    fs.rmSync(dogfoodLockPath, { force: true });
    return createDogfoodRunLock();
  }

  const detail = existing?.pid ? `pid ${existing.pid}` : "unknown process";
  throw new Error(
    `Another dogfood-local run is already active (${detail}). ` +
      `Wait for it to finish, or remove ${dogfoodLockPath} if the process is gone.`
  );
}

function createDogfoodRunLock() {
  const fd = fs.openSync(dogfoodLockPath, "wx");
  fs.writeFileSync(
    fd,
    `${JSON.stringify({ pid: process.pid, started_at: new Date().toISOString() })}\n`
  );
  cleanupTasks.push(() => {
    try {
      fs.closeSync(fd);
    } catch {
      // Ignore cleanup failures.
    }
    fs.rmSync(dogfoodLockPath, { force: true });
  });
}

function readDogfoodRunLock() {
  try {
    return JSON.parse(fs.readFileSync(dogfoodLockPath, "utf8"));
  } catch {
    return null;
  }
}

function processIsRunning(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function parseArgs(args) {
  const parsed = {
    list: false,
    doctor: false,
    reset: false,
    targets: [],
  };

  for (const arg of args) {
    if (arg === "--doctor") {
      parsed.doctor = true;
      continue;
    }
    if (arg === "--list") {
      parsed.list = true;
      continue;
    }
    if (arg === "--reset") {
      parsed.reset = true;
      continue;
    }
    parsed.targets.push(arg);
  }

  if (parsed.targets.length === 0) {
    parsed.targets.push("smoke");
  }

  return parsed;
}

function resolvePlaywrightArgs(targets) {
  const args = [];
  for (const target of targets) {
    const batch = batchMap[target];
    if (batch) {
      args.push(...batch);
      continue;
    }
    args.push(target);
  }
  return [...new Set(args)];
}

function printBatches() {
  console.log("Dogfood batches:");
  for (const [name, targets] of Object.entries(batchMap)) {
    console.log(`  ${name.padEnd(12)} ${targets.join(" ")}`);
  }
  console.log("");
  console.log("Utilities:");
  console.log("  --doctor     Check local tool and service prerequisites.");
  console.log(
    "  --reset      Remove managed Docker containers and auth state."
  );
}

async function runDoctor() {
  const checks = [];
  const addCheck = (name, ok, detail = "") => {
    checks.push({ name, ok, detail });
    const status = ok ? "ok" : "fail";
    console.log(`${status.padEnd(4)} ${name}${detail ? ` - ${detail}` : ""}`);
  };

  addCheck("bun", await commandAvailable("bun"));
  addCheck("go", await commandAvailable("go"));
  addCheck("infisical", await commandAvailable("infisical"));

  const postgresListening = await portListening(postgresEndpoint.port);
  const redisListening = await portListening(redisEndpoint.port);
  addCheck(
    "Postgres",
    postgresListening,
    `${postgresEndpoint.host}:${postgresEndpoint.port}`
  );
  addCheck(
    "Redis",
    redisListening,
    `${redisEndpoint.host}:${redisEndpoint.port}`
  );

  if (!(postgresListening && redisListening)) {
    const dockerAvailable = await dockerHealthy();
    addCheck(
      "Docker",
      dockerAvailable,
      dockerAvailable
        ? "available for managed Postgres/Redis"
        : "required when default Postgres/Redis ports are not listening"
    );
  }

  addCheck(
    "Strait API",
    await serviceHealthy(`${straitApiUrl}/health`),
    straitApiUrl
  );
  addCheck("Dashboard", await serviceHealthy(appUrl), appUrl);
  addCheck(
    "Sequin",
    await serviceHealthy(`${sequinBaseUrl}/health`),
    sequinBaseUrl
  );

  if (checks.some((check) => !check.ok)) {
    process.exitCode = 1;
  }
}

async function exportInfisicalEnv() {
  try {
    await run("infisical", [
      "export",
      "--env=dev",
      "--format=dotenv",
      `--output-file=${devVarsPath}`,
    ]);
  } catch (error) {
    console.log(
      "Infisical export failed; writing local-only dogfood environment."
    );
    if (error instanceof Error) {
      console.log(`  ${error.message}`);
    }
    writeLocalDogfoodEnv();
  }
}

function writeLocalDogfoodEnv() {
  const localEnv = {
    AUTH_DATABASE_URL: process.env.AUTH_DATABASE_URL || databaseUrl,
    BETTER_AUTH_SECRET:
      process.env.BETTER_AUTH_SECRET ||
      "dogfood-local-better-auth-secret-000000000000",
    BETTER_AUTH_URL: appUrl,
    DATABASE_URL: databaseUrl,
    E2E_LIMITED_USER_EMAIL:
      process.env.E2E_LIMITED_USER_EMAIL || "e2e-limited@strait.local",
    E2E_LIMITED_USER_PASSWORD:
      process.env.E2E_LIMITED_USER_PASSWORD ||
      process.env.E2E_USER_PASSWORD ||
      "dogfood-local-password",
    E2E_USER_EMAIL: process.env.E2E_USER_EMAIL || "e2e-owner@strait.local",
    E2E_USER_PASSWORD:
      process.env.E2E_USER_PASSWORD || "dogfood-local-password",
    ENCRYPTION_KEY: localEncryptionKey,
    INTERNAL_SECRET:
      process.env.INTERNAL_SECRET ||
      "dogfood-local-internal-secret-000000000000",
    JWT_SIGNING_KEY:
      process.env.JWT_SIGNING_KEY ||
      "dogfood-local-jwt-signing-key-000000000000",
    OIDC_ENABLED: "false",
    REDIS_URL: redisUrl,
    RESEND_API_KEY: "re_dogfood_local_dummy",
    RESEND_SUPPORT_EMAIL: "noreply@example.com",
    SEQUIN_API_TOKEN: "dogfood-local-sequin-token",
    SEQUIN_BASE_URL: sequinBaseUrl,
    SEQUIN_CONSUMER_NAME: "strait-cdc",
    STRAIT_API_URL: straitApiUrl,
    STRAIT_ENV: "development",
    STRIPE_SECRET_KEY: "",
    VITE_BASE_URL: appUrl,
    VITE_DISABLE_DEVTOOLS: "1",
    VITE_POSTHOG_HOST: "",
    VITE_POSTHOG_KEY: "",
    VITE_SENTRY_DSN: "",
    VITE_STRAIT_EDITION: "community",
  };

  fs.writeFileSync(
    devVarsPath,
    `${Object.entries(localEnv)
      .map(([key, value]) => `${key}=${quoteDotEnv(value)}`)
      .join("\n")}\n`
  );
}

function quoteDotEnv(value) {
  const escaped = String(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("\n", "\\n")
    .replaceAll('"', '\\"');
  return `"${escaped}"`;
}

async function buildStraitBinary() {
  fs.mkdirSync(dirname(binaryPath), { recursive: true });
  await run("go", ["build", "-p", "1", "-o", binaryPath, "./cmd/strait"], {
    cwd: straitDir,
    env: { ...process.env, GOMAXPROCS: process.env.GOMAXPROCS || "2" },
  });
}

async function buildDogfoodWorkerBinary() {
  fs.mkdirSync(dirname(workerBinaryPath), { recursive: true });
  await run(
    "go",
    ["build", "-p", "1", "-o", workerBinaryPath, "./cmd/dogfood-worker"],
    {
      cwd: straitDir,
      env: { ...process.env, GOMAXPROCS: process.env.GOMAXPROCS || "2" },
    }
  );
}

async function startBackend() {
  if (await serviceHealthy(`${straitApiUrl}/health`)) {
    console.log(`Reusing Strait backend at ${straitApiUrl}`);
    return;
  }

  const backend = spawn(binaryPath, ["--mode", "all"], {
    cwd: repoRoot,
    env: {
      ...process.env,
      ...readDotEnv(devVarsPath),
      DATABASE_URL: databaseUrl,
      ENCRYPTION_KEY: localEncryptionKey,
      REDIS_URL: redisUrl,
      PORT: apiPort,
      GRPC_PORT: grpcPort,
      STRAIT_API_URL: straitApiUrl,
      STRAIT_ENV: "development",
      CLICKHOUSE_EXPORT_ENABLED: "false",
      ALLOW_PRIVATE_ENDPOINTS: "true",
      WEBHOOK_REQUIRE_TLS: "false",
      RATE_LIMIT_REQUESTS: "100000",
      DEFAULT_API_KEY_RATE_LIMIT: "100000",
      DEFAULT_API_KEY_RATE_WINDOW_SECS: "60",
      TRIGGER_RATE_LIMIT_REQUESTS: "100000",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  backend.stdout.on("data", (chunk) => process.stdout.write(chunk));
  backend.stderr.on("data", (chunk) => process.stderr.write(chunk));
  cleanupTasks.push(() => stopProcess(backend));
  await waitForHealth(`${straitApiUrl}/health`, "Strait backend");
}

async function cleanupDogfoodDatabase() {
  if (process.env.DOGFOOD_SKIP_DB_CLEANUP === "1") {
    console.log("Skipping dogfood database cleanup.");
    return;
  }

  console.log("Cleaning prior dogfood-owned database rows...");
  const client = new Client({ connectionString: databaseUrl });
  await client.connect();
  try {
    await client.query("BEGIN");
    const counts = await client.query(`
      CREATE TEMP TABLE dogfood_target_jobs ON COMMIT DROP AS
      SELECT id
      FROM jobs
      WHERE name LIKE 'e2e-dogfood-%'
         OR name LIKE 'dogfood-%'
         OR slug LIKE 'e2e-dogfood-%'
         OR slug LIKE 'dogfood-%'
         OR queue_name LIKE 'dogfood-%';

      CREATE TEMP TABLE dogfood_target_runs ON COMMIT DROP AS
      SELECT id
      FROM job_runs
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);

      CREATE TEMP TABLE dogfood_target_workflows ON COMMIT DROP AS
      SELECT id
      FROM workflows
      WHERE name LIKE 'e2e-dogfood-%'
         OR name LIKE 'dogfood-%'
         OR slug LIKE 'e2e-dogfood-%'
         OR slug LIKE 'dogfood-%';

      CREATE TEMP TABLE dogfood_target_workflow_runs ON COMMIT DROP AS
      SELECT id
      FROM workflow_runs
      WHERE workflow_id IN (SELECT id FROM dogfood_target_workflows);

      CREATE TEMP TABLE dogfood_target_webhooks ON COMMIT DROP AS
      SELECT id
      FROM webhook_subscriptions
      WHERE webhook_url LIKE '%dogfood%';

      SELECT
        (SELECT COUNT(*)::INT FROM dogfood_target_jobs) AS jobs,
        (SELECT COUNT(*)::INT FROM dogfood_target_runs) AS runs,
        (SELECT COUNT(*)::INT FROM dogfood_target_workflows) AS workflows,
        (SELECT COUNT(*)::INT FROM dogfood_target_workflow_runs) AS workflow_runs,
        (SELECT COUNT(*)::INT FROM dogfood_target_webhooks) AS webhooks;
    `);
    await client.query(`
      DELETE FROM workflow_progression_event_claims
      WHERE event_id IN (
        SELECT id
        FROM workflow_progression_events
        WHERE workflow_run_id IN (SELECT id FROM dogfood_target_workflow_runs)
      );

      DELETE FROM workflow_progression_event_processed
      WHERE event_id IN (
        SELECT id
        FROM workflow_progression_events
        WHERE workflow_run_id IN (SELECT id FROM dogfood_target_workflow_runs)
      );

      DELETE FROM workflow_progression_events
      WHERE workflow_run_id IN (SELECT id FROM dogfood_target_workflow_runs);

      DELETE FROM webhook_deliveries
      WHERE subscription_id IN (SELECT id FROM dogfood_target_webhooks)
         OR run_id IN (SELECT id FROM dogfood_target_runs)
         OR job_id IN (SELECT id FROM dogfood_target_jobs)
         OR webhook_url LIKE '%dogfood%';

      DELETE FROM event_triggers
      WHERE workflow_run_id IN (SELECT id FROM dogfood_target_workflow_runs)
         OR job_run_id IN (SELECT id FROM dogfood_target_runs)
         OR event_key LIKE 'e2e-dogfood-%'
         OR event_key LIKE 'dogfood-%';

      DELETE FROM job_run_active_claims
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_lifecycle_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_ready_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_priority_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_visibility_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_cache_versions
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_heartbeats
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_terminal_state
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_retries
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_run_queue
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM strait_pgque_ready_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM queue_entries
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM enqueue_outbox
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM run_events
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM run_checkpoints
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM run_outputs
      WHERE run_id IN (SELECT id FROM dogfood_target_runs);

      DELETE FROM workflow_step_runs
      WHERE workflow_run_id IN (SELECT id FROM dogfood_target_workflow_runs);
      DELETE FROM workflow_runs
      WHERE id IN (SELECT id FROM dogfood_target_workflow_runs);

      DELETE FROM job_runs
      WHERE id IN (SELECT id FROM dogfood_target_runs);
      DELETE FROM job_versions
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM job_dependencies
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs)
         OR depends_on_job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM job_memory
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM job_slos
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM job_active_counts
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM batch_buffer
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM debounce_pending
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);
      DELETE FROM enqueue_outbox_history
      WHERE job_id IN (SELECT id FROM dogfood_target_jobs);

      DELETE FROM webhook_subscriptions
      WHERE id IN (SELECT id FROM dogfood_target_webhooks);
      DELETE FROM workflows
      WHERE id IN (SELECT id FROM dogfood_target_workflows);
      DELETE FROM jobs
      WHERE id IN (SELECT id FROM dogfood_target_jobs);
    `);
    await client.query("COMMIT");

    const summary = counts.at(-1)?.rows?.[0];
    if (summary) {
      console.log(
        [
          "Removed stale dogfood rows:",
          `${summary.jobs} jobs`,
          `${summary.runs} runs`,
          `${summary.workflows} workflows`,
          `${summary.workflow_runs} workflow runs`,
          `${summary.webhooks} webhooks`,
        ].join(" ")
      );
    }
  } catch (error) {
    await client.query("ROLLBACK").catch(() => undefined);
    throw new Error(
      `dogfood database cleanup failed: ${
        error instanceof Error ? error.message : String(error)
      }`
    );
  } finally {
    await client.end();
  }
}

function printRuntimeSummary() {
  console.log("");
  console.log("Dogfood runtime");
  console.log(`  App:      ${appUrl}`);
  console.log(`  API:      ${straitApiUrl}`);
  console.log(`  gRPC:     127.0.0.1:${grpcPort}`);
  console.log(`  Postgres: ${databaseUrl}`);
  console.log(`  Redis:    ${redisUrl}`);
  console.log(`  Batches:  ${options.targets.join(", ")}`);
  console.log("");
}

async function ensureLocalDependencies() {
  const postgresListening = await portListening(postgresEndpoint.port);
  const redisListening = await portListening(redisEndpoint.port);
  if (postgresListening && redisListening) {
    await ensurePostgres(true);
    await ensureRedis(true);
    return;
  }

  if (postgresEndpoint.port !== "15432" || redisEndpoint.port !== "16379") {
    throw new Error(
      [
        "Custom local Postgres or Redis ports are configured but not listening.",
        `Postgres: ${postgresEndpoint.host}:${postgresEndpoint.port}`,
        `Redis: ${redisEndpoint.host}:${redisEndpoint.port}`,
        "Start compatible local services on those ports, or unset DATABASE_URL/REDIS_URL to let the dogfood harness manage local dependencies.",
      ].join("\n")
    );
  }

  if (await nativeDependenciesAvailable()) {
    await ensureNativePostgres(postgresListening);
    await ensureNativeRedis(redisListening);
    return;
  }

  await ensureDockerAvailable();
  await ensurePostgres(postgresListening);
  await ensureRedis(redisListening);
}

async function resetLocalState() {
  const postgresName =
    process.env.E2E_POSTGRES_CONTAINER || "strait-app-e2e-postgres";
  const redisName = process.env.E2E_REDIS_CONTAINER || "strait-app-e2e-redis";
  if (await containerExists(postgresName)) {
    await run("docker", ["rm", "-f", postgresName], {
      stdio: "ignore",
      timeoutMs: 60_000,
    });
  }
  if (await containerExists(redisName)) {
    await run("docker", ["rm", "-f", redisName], {
      stdio: "ignore",
      timeoutMs: 60_000,
    });
  }
  fs.rmSync(resolve(appDir, "playwright/.auth"), {
    recursive: true,
    force: true,
  });
}

async function ensurePostgres(isListening = false) {
  const name = process.env.E2E_POSTGRES_CONTAINER || "strait-app-e2e-postgres";
  if (isListening) {
    console.log("Reusing Postgres on localhost:15432");
    return;
  }
  if (await containerExists(name)) {
    console.log(`Starting existing Postgres container ${name}`);
    await run("docker", ["start", name], { timeoutMs: 60_000 });
  } else {
    console.log(`Creating Postgres container ${name}`);
    await run(
      "docker",
      [
        "run",
        "-d",
        "--name",
        name,
        "-e",
        "POSTGRES_USER=strait",
        "-e",
        "POSTGRES_PASSWORD=strait",
        "-e",
        "POSTGRES_DB=strait",
        "-p",
        "15432:5432",
        "postgres:18",
      ],
      { timeoutMs: 60_000 }
    );
  }
  await waitForPostgres(name);
}

async function ensureRedis(isListening = false) {
  const name = process.env.E2E_REDIS_CONTAINER || "strait-app-e2e-redis";
  if (isListening) {
    console.log("Reusing Redis on localhost:16379");
    return;
  }
  if (await containerExists(name)) {
    console.log(`Starting existing Redis container ${name}`);
    await run("docker", ["start", name], { timeoutMs: 60_000 });
  } else {
    console.log(`Creating Redis container ${name}`);
    await run(
      "docker",
      ["run", "-d", "--name", name, "-p", "16379:6379", "redis:8-alpine"],
      { timeoutMs: 60_000 }
    );
  }
}

async function ensureLocalSequin() {
  if (await serviceHealthy(`${sequinBaseUrl}/health`)) {
    console.log(`Reusing Sequin at ${sequinBaseUrl}`);
    return;
  }

  if (!["localhost", "127.0.0.1", "::1"].includes(sequinEndpoint.host)) {
    throw new Error(
      `SEQUIN_BASE_URL is not reachable and cannot be stubbed: ${sequinBaseUrl}`
    );
  }

  console.log(`Starting local Sequin stub at ${sequinBaseUrl}`);
  const sinks = new Set(["strait-cdc"]);
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url || "/", sequinBaseUrl);
    const path = url.pathname;

    if (req.method === "GET" && path === "/health") {
      writeJSON(res, 200, { status: "ok" });
      return;
    }

    if (req.method === "GET" && path.startsWith("/api/sinks/")) {
      const name = decodeURIComponent(path.slice("/api/sinks/".length));
      sinks.add(name);
      writeJSON(res, 200, sequinSinkResponse(name));
      return;
    }

    if (req.method === "POST" && path === "/api/sinks") {
      const body = await readJSONBody(req);
      const name = body?.name || "strait-cdc";
      sinks.add(name);
      writeJSON(res, 201, sequinSinkResponse(name));
      return;
    }

    if (req.method === "POST" && sequinReceivePathPattern.test(path)) {
      writeJSON(res, 200, { data: [] });
      return;
    }

    if (req.method === "POST" && sequinAckPathPattern.test(path)) {
      writeJSON(res, 200, { ok: true });
      return;
    }

    writeJSON(res, 404, { error: `unknown sequin stub route ${path}` });
  });

  await new Promise((resolvePromise, reject) => {
    server.once("error", reject);
    server.listen(Number(sequinEndpoint.port), sequinEndpoint.host, () => {
      server.off("error", reject);
      resolvePromise();
    });
  });
  cleanupTasks.push(() => stopServer(server));
}

function sequinSinkResponse(name) {
  return {
    name,
    status: "active",
    health: { status: "waiting" },
  };
}

async function readJSONBody(req) {
  let raw = "";
  for await (const chunk of req) {
    raw += chunk;
  }
  if (!raw) {
    return {};
  }
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function writeJSON(res, statusCode, body) {
  res.writeHead(statusCode, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

async function nativeDependenciesAvailable() {
  return (
    (await findExecutable("postgres", postgresCandidates())) &&
    (await findExecutable("initdb", postgresCandidates("initdb"))) &&
    (await findExecutable("pg_isready", postgresCandidates("pg_isready"))) &&
    (await findExecutable("createdb", postgresCandidates("createdb"))) &&
    (await findExecutable("redis-server", redisCandidates()))
  );
}

function postgresCandidates(command = "postgres") {
  return [
    `/opt/homebrew/opt/postgresql@18/bin/${command}`,
    `/usr/local/opt/postgresql@18/bin/${command}`,
    `/opt/homebrew/opt/postgresql@17/bin/${command}`,
    `/usr/local/opt/postgresql@17/bin/${command}`,
    `/opt/homebrew/opt/postgresql@16/bin/${command}`,
    `/usr/local/opt/postgresql@16/bin/${command}`,
  ];
}

function redisCandidates(command = "redis-server") {
  return [
    `/opt/homebrew/opt/redis/bin/${command}`,
    `/usr/local/opt/redis/bin/${command}`,
  ];
}

async function findExecutable(command, candidates = []) {
  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  try {
    return (await capture("which", [command], { timeoutMs: 5000 })).trim();
  } catch {
    return "";
  }
}

async function ensureNativePostgres(isListening = false) {
  if (isListening) {
    console.log("Reusing Postgres on localhost:15432");
    return;
  }

  const postgres = await findExecutable("postgres", postgresCandidates());
  const initdb = await findExecutable("initdb", postgresCandidates("initdb"));
  const createdb = await findExecutable(
    "createdb",
    postgresCandidates("createdb")
  );

  fs.mkdirSync(dogfoodStateDir, { recursive: true });
  fs.mkdirSync(nativePostgresSocketDir, { recursive: true });
  if (!fs.existsSync(resolve(nativePostgresDataDir, "PG_VERSION"))) {
    console.log(`Initializing native Postgres at ${nativePostgresDataDir}`);
    const pwFile = resolve(dogfoodStateDir, "postgres.pw");
    fs.writeFileSync(pwFile, "strait\n", { mode: 0o600 });
    await run(
      initdb,
      [
        "-D",
        nativePostgresDataDir,
        "--username=strait",
        `--pwfile=${pwFile}`,
        "--auth=scram-sha-256",
        "--encoding=UTF8",
        "--locale=C",
      ],
      { timeoutMs: 60_000 }
    );
  }

  console.log("Starting native Postgres on localhost:15432");
  const postgresProcess = spawn(
    postgres,
    [
      "-D",
      nativePostgresDataDir,
      "-p",
      postgresEndpoint.port,
      "-k",
      nativePostgresSocketDir,
    ],
    {
      cwd: repoRoot,
      env: { ...process.env, LC_ALL: "C" },
      stdio: ["ignore", "pipe", "pipe"],
    }
  );
  postgresProcess.stdout.on("data", (chunk) => process.stdout.write(chunk));
  postgresProcess.stderr.on("data", (chunk) => process.stderr.write(chunk));
  cleanupTasks.push(() => stopProcess(postgresProcess));
  await waitForNativePostgres();
  await run(
    createdb,
    ["-h", "localhost", "-p", postgresEndpoint.port, "strait"],
    {
      env: { ...process.env, PGPASSWORD: "strait", PGUSER: "strait" },
      stdio: "ignore",
      timeoutMs: 30_000,
    }
  ).catch(() => undefined);
}

async function ensureNativeRedis(isListening = false) {
  if (isListening) {
    console.log("Reusing Redis on localhost:16379");
    return;
  }

  const redisServer = await findExecutable("redis-server", redisCandidates());
  fs.mkdirSync(nativeRedisDataDir, { recursive: true });

  console.log("Starting native Redis on localhost:16379");
  const redisProcess = spawn(
    redisServer,
    [
      "--port",
      redisEndpoint.port,
      "--dir",
      nativeRedisDataDir,
      "--save",
      "",
      "--appendonly",
      "no",
    ],
    {
      cwd: repoRoot,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    }
  );
  redisProcess.stdout.on("data", (chunk) => process.stdout.write(chunk));
  redisProcess.stderr.on("data", (chunk) => process.stderr.write(chunk));
  cleanupTasks.push(() => stopProcess(redisProcess));
  await waitForPort(redisEndpoint.port, "Redis");
}

function serviceEndpoint(value, fallbackHost, fallbackPort) {
  try {
    const url = new URL(value);
    return {
      host: url.hostname || fallbackHost,
      port: url.port || fallbackPort,
    };
  } catch {
    return { host: fallbackHost, port: fallbackPort };
  }
}

async function ensureDockerAvailable() {
  try {
    await checkDocker();
  } catch (error) {
    throw new Error(
      [
        "Docker is required to start local dogfood Postgres/Redis because localhost:15432 and localhost:16379 are not both listening.",
        "Start Docker Desktop, or start compatible local services on those ports, then rerun `bun run dogfood -- smoke`.",
        error instanceof Error ? `Docker check failed: ${error.message}` : "",
      ]
        .filter(Boolean)
        .join("\n")
    );
  }
}

async function dockerHealthy() {
  try {
    await checkDocker();
    return true;
  } catch {
    return false;
  }
}

async function checkDocker() {
  await capture("docker", ["info", "--format", "{{.ServerVersion}}"], {
    timeoutMs: 15_000,
  });
}

async function commandAvailable(command) {
  try {
    await capture("which", [command], { timeoutMs: 5000 });
    return true;
  } catch {
    return false;
  }
}

async function containerExists(name) {
  const output = await capture(
    "docker",
    ["ps", "-a", "--filter", `name=${name}`, "--format", "{{.Names}}"],
    { timeoutMs: 20_000 }
  );
  return output
    .split("\n")
    .map((line) => line.trim())
    .includes(name);
}

async function portListening(port) {
  try {
    await run("lsof", ["-nP", `-iTCP:${port}`, "-sTCP:LISTEN"], {
      stdio: "ignore",
      timeoutMs: 10_000,
    });
    return true;
  } catch {
    return false;
  }
}

async function waitForPostgres(containerName) {
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline) {
    try {
      await run(
        "docker",
        ["exec", containerName, "pg_isready", "-U", "strait", "-d", "strait"],
        { stdio: "ignore", timeoutMs: 10_000 }
      );
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }
  throw new Error(`Postgres did not become ready in ${containerName}`);
}

async function waitForNativePostgres() {
  const pgIsReady = await findExecutable(
    "pg_isready",
    postgresCandidates("pg_isready")
  );
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline) {
    try {
      await run(
        pgIsReady,
        ["-h", "localhost", "-p", postgresEndpoint.port, "-U", "strait"],
        {
          env: { ...process.env, PGPASSWORD: "strait" },
          stdio: "ignore",
          timeoutMs: 10_000,
        }
      );
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }
  throw new Error("Native Postgres did not become ready on localhost:15432");
}

async function waitForPort(port, label) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    if (await portListening(port)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`${label} did not start listening on port ${port}`);
}

async function serviceHealthy(url) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 3000);
  try {
    const response = await fetch(url, { signal: controller.signal });
    return response.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

async function waitForHealth(url, label) {
  const deadline = Date.now() + 120_000;
  while (Date.now() < deadline) {
    if (await serviceHealthy(url)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`${label} did not become healthy at ${url}`);
}

function readDotEnv(path) {
  if (!fs.existsSync(path)) {
    return {};
  }

  const env = {};
  for (const rawLine of fs.readFileSync(path, "utf-8").split("\n")) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    const eq = line.indexOf("=");
    if (eq === -1) {
      continue;
    }
    const key = line.slice(0, eq);
    let value = line.slice(eq + 1);
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    env[key] = value;
  }
  return env;
}

function run(command, args, options = {}) {
  return new Promise((resolvePromise, reject) => {
    let settled = false;
    const settle = (callback, value) => {
      if (settled) {
        return;
      }
      settled = true;
      if (timer) {
        clearTimeout(timer);
      }
      callback(value);
    };
    const child = spawn(command, args, {
      cwd: options.cwd || repoRoot,
      env: options.env || process.env,
      stdio: options.stdio || "inherit",
    });
    const timer = options.timeoutMs
      ? setTimeout(() => {
          terminateChild(child);
          settle(
            reject,
            new Error(
              `${command} ${args.join(" ")} timed out after ${options.timeoutMs}ms`
            )
          );
        }, options.timeoutMs)
      : null;
    child.on("error", (error) => settle(reject, error));
    child.on("exit", (code, signal) => {
      if (code === 0) {
        settle(resolvePromise);
        return;
      }
      settle(
        reject,
        new Error(
          `${command} ${args.join(" ")} failed${
            signal ? ` (${signal})` : ` with exit code ${code}`
          }`
        )
      );
    });
  });
}

function capture(command, args, options = {}) {
  return new Promise((resolvePromise, reject) => {
    let settled = false;
    const settle = (callback, value) => {
      if (settled) {
        return;
      }
      settled = true;
      if (timer) {
        clearTimeout(timer);
      }
      callback(value);
    };
    const child = spawn(command, args, {
      cwd: repoRoot,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const timer = options.timeoutMs
      ? setTimeout(() => {
          terminateChild(child);
          settle(
            reject,
            new Error(
              `${command} ${args.join(" ")} timed out after ${options.timeoutMs}ms`
            )
          );
        }, options.timeoutMs)
      : null;
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => settle(reject, error));
    child.on("exit", (code) => {
      if (code === 0) {
        settle(resolvePromise, stdout);
        return;
      }
      settle(
        reject,
        new Error(stderr || `${command} ${args.join(" ")} failed`)
      );
    });
  });
}

function terminateChild(child) {
  child.kill("SIGTERM");
  child.stdout?.destroy();
  child.stderr?.destroy();
  setTimeout(() => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill("SIGKILL");
    }
  }, 1000).unref();
}

async function stopProcess(child) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  child.kill("SIGINT");
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      resolvePromise();
    }, 10_000);
    child.once("exit", () => {
      clearTimeout(timer);
      resolvePromise();
    });
  });
}

function stopServer(server) {
  return new Promise((resolvePromise) => {
    server.close(() => resolvePromise());
  });
}

async function cleanup() {
  while (cleanupTasks.length > 0) {
    const task = cleanupTasks.pop();
    await Promise.resolve(task()).catch(() => undefined);
  }
}
