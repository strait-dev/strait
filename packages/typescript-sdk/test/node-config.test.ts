import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  createClientFromConfigFile,
  defineStraitConfig,
  findStraitConfigFile,
  loadStraitConfig,
} from "../src/node/index";
import type { FetchLike } from "../src/runtime";

const tempDirs: string[] = [];

const createTempDir = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-ts-config-"));
  tempDirs.push(directory);
  return directory;
};

afterEach(async () => {
  await Promise.all(
    tempDirs
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

describe("node config helpers", () => {
  test("defineStraitConfig returns typed input unchanged", () => {
    const config = defineStraitConfig({
      baseUrl: "https://strait.dev",
      auth: { type: "bearer", token: "abc" as string },
    });

    expect(config.baseUrl).toBe("https://strait.dev");
    expect(config.auth.type).toBe("bearer");
  });

  test("findStraitConfigFile discovers strait.config.ts", async () => {
    const directory = await createTempDir();
    const configPath = join(directory, "strait.config.ts");

    await writeFile(
      configPath,
      'export default { baseUrl: "https://strait.dev", auth: { type: "bearer", token: "abc" } };\n'
    );

    const discovered = await findStraitConfigFile({ cwd: directory });

    expect(discovered).toBe(configPath);
  });

  test("loadStraitConfig loads and normalizes default exported config", async () => {
    const directory = await createTempDir();
    await writeFile(
      join(directory, "strait.config.ts"),
      'export default { baseUrl: "https://strait.dev/", auth: { type: "runToken", token: "rt_123" } };\n'
    );

    const loaded = await loadStraitConfig({ cwd: directory });

    expect(loaded.baseUrl).toBe("https://strait.dev");
    expect(loaded.auth.type).toBe("runToken");
  });

  test("loadStraitConfig supports named config export and client wrapper", async () => {
    const directory = await createTempDir();
    await writeFile(
      join(directory, "strait.config.ts"),
      'export const config = () => ({ client: { baseUrl: "https://strait.dev", auth: { type: "apiKey", token: "key_123" } } });\n'
    );

    const loaded = await loadStraitConfig({ cwd: directory });

    expect(loaded.baseUrl).toBe("https://strait.dev");
    expect(loaded.auth.type).toBe("apiKey");
  });

  test("findStraitConfigFile discovers strait.json first", async () => {
    const directory = await createTempDir();
    await writeFile(
      join(directory, "strait.json"),
      JSON.stringify({
        project: { id: "proj_1" },
        sdk: { base_url: "https://strait.dev" },
      })
    );
    await writeFile(
      join(directory, "strait.config.ts"),
      'export default { baseUrl: "https://strait.dev", auth: { type: "bearer", token: "abc" } };\n'
    );

    const discovered = await findStraitConfigFile({ cwd: directory });
    expect(discovered).toBe(join(directory, "strait.json"));
  });

  test("loadStraitConfig loads strait.json and maps sdk fields", async () => {
    const directory = await createTempDir();
    const env = process.env;
    process.env = { ...env, STRAIT_API_KEY: "key_from_env" };

    try {
      await writeFile(
        join(directory, "strait.json"),
        JSON.stringify({
          project: { id: "proj_1" },
          sdk: {
            base_url: "https://api.strait.dev/",
            auth_type: "bearer",
            timeout_ms: 5000,
          },
        })
      );

      const loaded = await loadStraitConfig({
        cwd: directory,
        envOverrides: false,
      });
      expect(loaded.baseUrl).toBe("https://api.strait.dev");
      expect(loaded.auth.type).toBe("bearer");
      expect(loaded.auth.token).toBe("key_from_env");
      expect(loaded.timeoutMs).toBe(5000);
    } finally {
      process.env = env;
    }
  });

  test("loadStraitConfig applies env overrides on top of strait.json", async () => {
    const directory = await createTempDir();
    const env = process.env;
    process.env = {
      ...env,
      STRAIT_BASE_URL: "https://env.example.com",
      STRAIT_API_KEY: "env_key",
      STRAIT_AUTH_TYPE: "runToken",
      STRAIT_TIMEOUT_MS: "9000",
    };

    try {
      await writeFile(
        join(directory, "strait.json"),
        JSON.stringify({
          sdk: {
            base_url: "https://file.example.com",
            auth_type: "apiKey",
            timeout_ms: 5000,
          },
        })
      );

      const loaded = await loadStraitConfig({ cwd: directory });
      expect(loaded.baseUrl).toBe("https://env.example.com");
      expect(loaded.auth.type).toBe("runToken");
      expect(loaded.auth.token).toBe("env_key");
      expect(loaded.timeoutMs).toBe(9000);
    } finally {
      process.env = env;
    }
  });

  test("loadStraitConfig emits deprecation warning for .ts config", async () => {
    const directory = await createTempDir();
    await writeFile(
      join(directory, "strait.config.ts"),
      'export default { baseUrl: "https://strait.dev", auth: { type: "bearer", token: "abc" } };\n'
    );

    const warns: string[] = [];
    const origWarn = console.warn;
    console.warn = (...args: unknown[]) => warns.push(String(args[0]));

    try {
      await loadStraitConfig({ cwd: directory });
      expect(warns.length).toBeGreaterThan(0);
      expect(warns[0]).toContain("deprecated");
    } finally {
      console.warn = origWarn;
    }
  });

  test("createClientFromConfigFile boots client using discovered file", async () => {
    const directory = await createTempDir();
    await writeFile(
      join(directory, "strait.config.ts"),
      'export default { baseUrl: "https://strait.dev/", auth: { type: "runToken", token: "rt_123" } };\n'
    );

    let capturedUrl = "";
    let capturedRequest: RequestInit | undefined;

    const fetchImpl: FetchLike = (input, init) => {
      capturedUrl = String(input);
      capturedRequest = init;
      return Promise.resolve(
        new Response(JSON.stringify({ ok: true }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        })
      );
    };

    const client = await createClientFromConfigFile({
      cwd: directory,
      fetch: fetchImpl,
    });

    const response = await client.logRun({
      pathParams: { runID: "run-1" },
      body: { message: "hello" },
      successStatus: [201],
    });

    expect(response).toEqual({ ok: true });
    expect(capturedUrl).toBe("https://strait.dev/sdk/v1/runs/run-1/log");
    expect(
      (capturedRequest?.headers as Record<string, string>).Authorization
    ).toBe("Bearer rt_123");
  });
});
