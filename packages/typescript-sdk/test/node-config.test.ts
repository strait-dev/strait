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
