import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { resolveSentryRelease } from "../sentry-release";
import { type CliRunner, uploadSourcemaps } from "../sentry-sourcemaps";

const tempDirs: string[] = [];

afterEach(async () => {
  await Promise.all(
    tempDirs.map((dir) => rm(dir, { force: true, recursive: true }))
  );
  tempDirs.length = 0;
});

describe("resolveSentryRelease", () => {
  it("derives release from STRAIT_VERSION and STRAIT_COMMIT", () => {
    expect(
      resolveSentryRelease({
        STRAIT_VERSION: "v1.2.3",
        STRAIT_COMMIT: "abcdef1234567890",
      })
    ).toBe("v1.2.3+abcdef123456");
  });

  it("prefers explicit SENTRY_RELEASE", () => {
    expect(
      resolveSentryRelease({
        SENTRY_RELEASE: "dashboard@v1.2.3",
        STRAIT_VERSION: "v1.2.3",
        STRAIT_COMMIT: "abcdef",
      })
    ).toBe("dashboard@v1.2.3");
  });
});

describe("uploadSourcemaps", () => {
  it("uploads all build sourcemaps and removes public maps", async () => {
    const cwd = await makeTempDir();
    await writeFixture(cwd, "dist/client/assets/app.js.map");
    await writeFixture(cwd, "dist/server/server.js.map");
    await writeFixture(cwd, ".output/public/chunk.js.map");
    await writeFixture(cwd, ".output/server/worker.js.map");

    const calls: Array<{ command: string; args: string[] }> = [];
    const runner: CliRunner = (command, args) => {
      calls.push({ command, args });
    };

    const result = await uploadSourcemaps({
      cwd,
      env: {
        ...process.env,
        STRAIT_VERSION: "v1.2.3",
        STRAIT_COMMIT: "abcdef1234567890",
      },
      runner,
    });

    expect(result.release).toBe("v1.2.3+abcdef123456");
    expect(result.uploaded).toEqual([
      ".output/public/chunk.js.map",
      ".output/server/worker.js.map",
      "dist/client/assets/app.js.map",
      "dist/server/server.js.map",
    ]);
    expect(calls).toHaveLength(4);
    expect(calls.every((call) => call.command === "sentry-cli")).toBe(true);
    expect(calls.map((call) => call.args.slice(0, 5))).toEqual(
      result.uploaded.map((file) => [
        "releases",
        "files",
        "v1.2.3+abcdef123456",
        "upload-sourcemaps",
        file,
      ])
    );
    expect(result.removed).toEqual([
      ".output/public/chunk.js.map",
      "dist/client/assets/app.js.map",
    ]);

    await expect(
      readFile(path.join(cwd, "dist/client/assets/app.js.map"), "utf8")
    ).rejects.toThrow();
    await expect(
      readFile(path.join(cwd, ".output/public/chunk.js.map"), "utf8")
    ).rejects.toThrow();
    await expect(
      readFile(path.join(cwd, "dist/server/server.js.map"), "utf8")
    ).resolves.toContain("source");
  });
});

async function makeTempDir(): Promise<string> {
  const dir = await mkdtemp(path.join(tmpdir(), "strait-sentry-"));
  tempDirs.push(dir);
  return dir;
}

async function writeFixture(cwd: string, relativePath: string) {
  const absolutePath = path.join(cwd, relativePath);
  await mkdir(path.dirname(absolutePath), { recursive: true });
  await writeFile(absolutePath, '{"version":3,"sources":["source.ts"]}');
}
