import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, devices } from "@playwright/test";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const appDir = resolve(__dirname, "..");
const devVarsPath = resolve(appDir, ".dev.vars");
const isCI = !!process.env.CI;
const useDogfoodLocalEnv = process.env.DOGFOOD_LOCAL_ENV === "1";
const localEnvOverrideKeys = [
  "AUTH_DATABASE_URL",
  "DATABASE_URL",
  "REDIS_URL",
  "STRAIT_API_URL",
] as const;
const defaultBaseUrl = "http://localhost:5173";
const webServerUrl = "http://127.0.0.1:5173";
const webServerStartupTimeoutMs = 240_000;

/** Quote a value for the shell command Playwright uses to launch Vite. */
function shellQuote(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

/** Collect local service overrides passed through the outer Infisical command. */
function localEnvOverrides() {
  const overrides: Record<string, string> = {};
  for (const key of localEnvOverrideKeys) {
    const value = process.env[key];
    if (value) {
      overrides[key] = value;
    }
  }
  return Object.entries(overrides);
}

const envOverrides = localEnvOverrides();
const envPrefix =
  envOverrides.length > 0
    ? `env ${envOverrides
        .map(([key, value]) => `${key}=${shellQuote(value)}`)
        .join(" ")}`
    : "env";
const devVarsOverrideCommand = envOverrides
  .map(
    ([key, value]) =>
      `printf '\\n${key}=%s\\n' ${shellQuote(value)} >> ${shellQuote(devVarsPath)}`
  )
  .join(" && ");
const sourceDevVarsCommand = `set -a && . ${shellQuote(devVarsPath)} && set +a`;
const webServerCommand = useDogfoodLocalEnv
  ? [
      `cd ${shellQuote(appDir)}`,
      `${sourceDevVarsCommand} && ${envPrefix} bun run db:migrate:bun`,
      `${sourceDevVarsCommand} && DISABLE_NGROK=1 ${envPrefix} VITE_DISABLE_DEVTOOLS=1 bun run dev --host 127.0.0.1`,
    ]
      .filter(Boolean)
      .join(" && ")
  : [
      `cd ${shellQuote(appDir)}`,
      `infisical export --env=dev --format=dotenv --output-file=${shellQuote(devVarsPath)} >/dev/null`,
      devVarsOverrideCommand,
      `infisical run --env=dev -- ${envPrefix} bun run db:migrate:bun`,
      `DISABLE_NGROK=1 infisical run --env=dev -- ${envPrefix} VITE_DISABLE_DEVTOOLS=1 bun run dev --host 127.0.0.1`,
    ]
      .filter(Boolean)
      .join(" && ");

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: isCI ? 2 : 1,
  maxFailures: isCI ? 20 : undefined,
  reporter: isCI ? [["html"], ["github"]] : "html",
  timeout: 30_000,
  expect: {
    timeout: 8000,
  },
  globalSetup: resolve(__dirname, "global-setup.ts"),
  globalTeardown: resolve(__dirname, "global-teardown.ts"),
  use: {
    baseURL: process.env.EXPECT_BASE_URL || defaultBaseUrl,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    storageState: resolve(__dirname, "../playwright/.auth/user.json"),
    navigationTimeout: 30_000,
    actionTimeout: 10_000,
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: isCI
    ? undefined
    : {
        command: webServerCommand,
        url: webServerUrl,
        reuseExistingServer: true,
        timeout: webServerStartupTimeoutMs,
      },
});
