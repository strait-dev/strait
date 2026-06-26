import { spawn } from "node:child_process";
import fs from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const appDir = resolve(__dirname, "..");
const repoRoot = resolve(appDir, "../..");
const straitDir = resolve(repoRoot, "apps/strait");
const binaryPath = resolve(repoRoot, ".turbo/strait-e2e/strait");
const devVarsPath = resolve(appDir, ".dev.vars");
const envFileVars = {
  ...readDotEnv(resolve(repoRoot, ".env")),
  ...readDotEnv(resolve(appDir, ".env")),
  ...readDotEnv(devVarsPath),
};
const runtimeEnv = { ...envFileVars, ...process.env };

const apiPort = runtimeEnv.E2E_STRAIT_PORT || "18082";
const grpcPort = runtimeEnv.E2E_STRAIT_GRPC_PORT || "15053";
const databaseUrl =
  runtimeEnv.DATABASE_URL ||
  "postgres://strait:strait@localhost:15432/strait?sslmode=disable";
const redisUrl = runtimeEnv.REDIS_URL || "redis://localhost:16379";
const straitApiUrl = runtimeEnv.STRAIT_API_URL || `http://localhost:${apiPort}`;
const defaultPlaywrightArgs = ["tests/harness", "tests/core-dashboard"];
const playwrightArgs =
  process.argv.length > 2 ? process.argv.slice(2) : defaultPlaywrightArgs;

const cleanupTasks = [];

main().catch(async (error) => {
  await cleanup();
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});

async function main() {
  for (const signal of ["SIGINT", "SIGTERM"]) {
    process.once(signal, async () => {
      await cleanup();
      process.kill(process.pid, signal);
    });
  }

  await ensureLocalDependencies();

  fs.mkdirSync(dirname(binaryPath), { recursive: true });
  await run("go", ["build", "-p", "1", "-o", binaryPath, "./cmd/strait"], {
    cwd: straitDir,
    env: { ...runtimeEnv, GOMAXPROCS: runtimeEnv.GOMAXPROCS || "2" },
  });

  const backend = spawn(binaryPath, ["--mode", "all"], {
    cwd: repoRoot,
    env: {
      ...runtimeEnv,
      DATABASE_URL: databaseUrl,
      REDIS_URL: redisUrl,
      PORT: apiPort,
      GRPC_PORT: grpcPort,
      STRAIT_API_URL: straitApiUrl,
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

  await run("bun", ["run", "e2e", "--", ...playwrightArgs], {
    cwd: appDir,
    env: {
      ...runtimeEnv,
      DATABASE_URL: databaseUrl,
      AUTH_DATABASE_URL: runtimeEnv.AUTH_DATABASE_URL || databaseUrl,
      REDIS_URL: redisUrl,
      STRAIT_API_URL: straitApiUrl,
      EXPECT_BASE_URL: runtimeEnv.EXPECT_BASE_URL || "http://localhost:5173",
    },
  });

  await cleanup();
}

async function ensureLocalDependencies() {
  await ensurePostgres();
  await ensureRedis();
}

async function containerExists(name) {
  const output = await capture("docker", [
    "ps",
    "-a",
    "--filter",
    `name=${name}`,
    "--format",
    "{{.Names}}",
  ]);
  return output
    .split("\n")
    .map((line) => line.trim())
    .includes(name);
}

async function portListening(port) {
  try {
    await run("lsof", ["-nP", `-iTCP:${port}`, "-sTCP:LISTEN"], {
      stdio: "ignore",
    });
    return true;
  } catch {
    return false;
  }
}

async function ensurePostgres() {
  const name = process.env.E2E_POSTGRES_CONTAINER || "strait-app-e2e-postgres";
  const reusePostgres = process.env.E2E_REUSE_POSTGRES === "1";
  if (!reusePostgres && (await containerExists(name))) {
    await run("docker", ["rm", "-f", name], { stdio: "ignore" });
  }
  if (await portListening("15432")) {
    if (!reusePostgres) {
      throw new Error(
        "Port 15432 is already in use. Stop the existing service or set E2E_REUSE_POSTGRES=1 to reuse it."
      );
    }
    return;
  }
  if (reusePostgres && (await containerExists(name))) {
    await run("docker", ["start", name]);
  } else {
    await run("docker", [
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
      "postgres:15",
    ]);
  }
  const cleanupArgs = reusePostgres ? ["stop", name] : ["rm", "-f", name];
  cleanupTasks.push(() =>
    run("docker", cleanupArgs, { stdio: "ignore" }).catch(() => undefined)
  );
  await waitForPostgres(name);
}

async function ensureRedis() {
  const name = process.env.E2E_REDIS_CONTAINER || "strait-app-e2e-redis";
  if (await portListening("16379")) {
    return;
  }
  if (await containerExists(name)) {
    await run("docker", ["start", name]);
  } else {
    await run("docker", [
      "run",
      "-d",
      "--name",
      name,
      "-p",
      "16379:6379",
      "redis:8-alpine",
    ]);
  }
  cleanupTasks.push(() =>
    run("docker", ["stop", name], { stdio: "ignore" }).catch(() => undefined)
  );
}

async function waitForPostgres(containerName) {
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline) {
    try {
      await run(
        "docker",
        ["exec", containerName, "pg_isready", "-U", "strait", "-d", "strait"],
        { stdio: "ignore" }
      );
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }
  throw new Error(`Postgres did not become ready in ${containerName}`);
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

async function waitForHealth(url, label) {
  const deadline = Date.now() + 120_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Service is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`${label} did not become healthy at ${url}`);
}

function run(command, args, options = {}) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd || repoRoot,
      env: options.env || process.env,
      stdio: options.stdio || "inherit",
    });
    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (code === 0) {
        resolvePromise();
        return;
      }
      reject(
        new Error(
          `${command} ${args.join(" ")} failed${signal ? ` (${signal})` : ` with exit code ${code}`}`
        )
      );
    });
  });
}

function capture(command, args) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, {
      cwd: repoRoot,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolvePromise(stdout);
        return;
      }
      reject(new Error(stderr || `${command} ${args.join(" ")} failed`));
    });
  });
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

async function cleanup() {
  while (cleanupTasks.length > 0) {
    const task = cleanupTasks.pop();
    await task().catch(() => undefined);
  }
}
