import { setTimeout as sleep } from "node:timers/promises";
import { getMigrations } from "better-auth/db/migration";
import pg from "pg";
import {
  DEFAULT_LOCAL_DEV_USER_EMAIL,
  DEFAULT_LOCAL_DEV_USER_NAME,
  DEFAULT_LOCAL_DEV_USER_PASSWORD,
} from "./local-auth-user";

const LOCAL_AUTH_HOST_FALLBACK = "localhost";
const LOCAL_APP_PORT_FALLBACK = "5173";
const BOOTSTRAP_TIMEOUT_MS = 30_000;

export const LOCAL_DEFAULTS: Record<string, string> = {
  AUTH_DATABASE_URL: "postgresql://strait:strait@localhost:5432/strait",
  BETTER_AUTH_SECRET: "strait-local-better-auth-secret-32chars",
  DISABLE_EMAIL_VERIFICATION: "true",
  DISABLE_POLAR_BILLING: "true",
  INTERNAL_SECRET: "strait-local-internal-secret-32chars",
  LOCAL_DEV_USER_EMAIL: DEFAULT_LOCAL_DEV_USER_EMAIL,
  LOCAL_DEV_USER_PASSWORD: DEFAULT_LOCAL_DEV_USER_PASSWORD,
  LOCAL_DEV_USER_NAME: DEFAULT_LOCAL_DEV_USER_NAME,
  STRAIT_API_URL: "http://127.0.0.1:8080",
};

const REQUIRED_AUTH_TABLES = [
  "user",
  "account",
  "session",
  "organization",
  "member",
  "invitation",
  "passkey",
  "jwks",
  "oauthClient",
  "oauthRefreshToken",
  "oauthAccessToken",
  "oauthConsent",
] as const;

const REQUIRED_AUTH_COLUMNS = {
  user: ["twoFactorEnabled", "defaultOrganizationId", "activeProjectId"],
  session: ["activeOrganizationId"],
  invitation: ["createdAt"],
  passkey: ["credentialID", "aaguid"],
} as const;

type AuthMigrationSummary = {
  toBeCreated: { table: string }[];
  toBeAdded: { table: string; fields?: Record<string, unknown> }[];
};

type DevServerOptions = {
  host: string;
  port: string;
};

function normalizePublicHost(host: string) {
  if (host === "0.0.0.0" || host === "::") {
    return LOCAL_AUTH_HOST_FALLBACK;
  }
  return host;
}

export function parseDevServerOptions(args: string[]): DevServerOptions {
  let host = LOCAL_AUTH_HOST_FALLBACK;
  let port = LOCAL_APP_PORT_FALLBACK;

  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === "--host" && args[i + 1]) {
      host = args[i + 1];
      i += 1;
      continue;
    }
    if (arg.startsWith("--host=")) {
      host = arg.slice("--host=".length);
      continue;
    }
    if (arg === "--port" && args[i + 1]) {
      port = args[i + 1];
      i += 1;
      continue;
    }
    if (arg.startsWith("--port=")) {
      port = arg.slice("--port=".length);
    }
  }

  return {
    host: normalizePublicHost(host),
    port,
  };
}

export function applyLocalDefaults(
  source: NodeJS.ProcessEnv,
  args: string[]
): NodeJS.ProcessEnv {
  const env = { ...source };
  const { host, port } = parseDevServerOptions(args);
  const derivedBaseURL = `http://${host}:${port}`;

  for (const [key, value] of Object.entries(LOCAL_DEFAULTS)) {
    if (!env[key]) {
      env[key] = value;
    }
  }

  if (!env.BETTER_AUTH_URL) {
    env.BETTER_AUTH_URL = derivedBaseURL;
  }
  if (!env.VITE_BASE_URL) {
    env.VITE_BASE_URL = derivedBaseURL;
  }

  return env;
}

async function withPool<T>(
  connectionString: string,
  fn: (pool: pg.Pool) => Promise<T>
) {
  const pool = new pg.Pool({ connectionString });
  try {
    return await fn(pool);
  } finally {
    await pool.end();
  }
}

export async function waitForDatabase(
  connectionString: string,
  timeoutMs = BOOTSTRAP_TIMEOUT_MS
) {
  const startedAt = Date.now();
  let lastError: unknown;

  while (Date.now() - startedAt < timeoutMs) {
    try {
      await withPool(connectionString, async (pool) => {
        await pool.query("select 1");
      });
      return;
    } catch (error) {
      lastError = error;
      await sleep(500);
    }
  }

  throw new Error(
    `Timed out waiting for PostgreSQL at ${connectionString}: ${String(lastError)}`
  );
}

export async function waitForBaseURL(
  baseURL: string,
  timeoutMs = BOOTSTRAP_TIMEOUT_MS
) {
  const startedAt = Date.now();
  let lastError: unknown;

  while (Date.now() - startedAt < timeoutMs) {
    try {
      const response = await fetch(new URL("/login", baseURL), {
        redirect: "manual",
      });
      const contentType = response.headers.get("content-type") ?? "";

      if (response.ok && contentType.includes("text/html")) {
        return;
      }

      lastError = new Error(
        `received HTTP ${response.status} with content-type ${contentType || "<empty>"}`
      );
    } catch (error) {
      lastError = error;
    }

    await sleep(500);
  }

  throw new Error(
    `Timed out waiting for app at ${baseURL}: ${String(lastError)}`
  );
}

async function patchKnownMigrationCompatibility(connectionString: string) {
  await withPool(connectionString, async (pool) => {
    await pool.query(`
      DO $$
      BEGIN
        IF EXISTS (
          SELECT 1 FROM information_schema.tables
          WHERE table_schema = 'public' AND table_name = 'passkey'
        ) THEN
          EXECUTE 'ALTER TABLE passkey ADD COLUMN IF NOT EXISTS "credentialID" text';
          EXECUTE 'ALTER TABLE passkey ADD COLUMN IF NOT EXISTS "aaguid" text';
        END IF;
      END $$;
    `);
  });
}

export async function getMissingAuthSchema(connectionString: string) {
  return await withPool(connectionString, async (pool) => {
    const missing: string[] = [];

    for (const table of REQUIRED_AUTH_TABLES) {
      const exists = await pool.query<{ exists: boolean }>(
        `
          SELECT EXISTS (
            SELECT 1
            FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = $1
          ) AS exists
        `,
        [table]
      );
      if (!exists.rows[0]?.exists) {
        missing.push(`table:${table}`);
      }
    }

    for (const [table, columns] of Object.entries(REQUIRED_AUTH_COLUMNS)) {
      for (const column of columns) {
        const exists = await pool.query<{ exists: boolean }>(
          `
            SELECT EXISTS (
              SELECT 1
              FROM information_schema.columns
              WHERE table_schema = 'public'
                AND table_name = $1
                AND column_name = $2
            ) AS exists
          `,
          [table, column]
        );
        if (!exists.rows[0]?.exists) {
          missing.push(`column:${table}.${column}`);
        }
      }
    }

    return missing;
  });
}

function logMigrationSummary(summary: AuthMigrationSummary) {
  const { toBeAdded, toBeCreated } = summary;

  if (toBeCreated.length === 0 && toBeAdded.length === 0) {
    console.log("No pending Better Auth migrations. Database schema is up to date.");
    return;
  }

  if (toBeCreated.length > 0) {
    console.log("Tables to create:");
    for (const table of toBeCreated) {
      console.log(`  + ${table.table}`);
    }
    console.log();
  }

  if (toBeAdded.length > 0) {
    console.log("Columns to add:");
    for (const column of toBeAdded) {
      const fields = column.fields ? Object.keys(column.fields).join(", ") : "unknown";
      console.log(`  + ${column.table}.${fields}`);
    }
    console.log();
  }
}

export async function migrateAuthDatabase(dryRun = false) {
  const connectionString = process.env.AUTH_DATABASE_URL;
  if (!connectionString) {
    throw new Error("AUTH_DATABASE_URL is required for local auth bootstrap");
  }

  await waitForDatabase(connectionString);
  await patchKnownMigrationCompatibility(connectionString);
  const { auth } = await import("../../src/lib/auth.server");

  const { toBeAdded, toBeCreated, runMigrations } = await getMigrations(
    auth.options
  );
  const summary = { toBeAdded, toBeCreated };
  logMigrationSummary(summary);

  if (!dryRun && (toBeAdded.length > 0 || toBeCreated.length > 0)) {
    console.log("Applying Better Auth migrations...");
    await runMigrations();
    await patchKnownMigrationCompatibility(connectionString);
  }

  const missing = await getMissingAuthSchema(connectionString);
  if (missing.length > 0) {
    throw new Error(
      `Local auth schema is incomplete after migration: ${missing.join(", ")}`
    );
  }

  if (!dryRun) {
    console.log("Better Auth schema is ready for local development.");
  }

  return summary;
}
