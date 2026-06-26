import { spawnSync } from "node:child_process";
import { readdir, rm, stat } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { type ReleaseEnv, resolveSentryRelease } from "./sentry-release";

const BUILD_ROOTS = ["dist", ".output"];
const PUBLIC_ROOTS = ["dist/client", ".output/public", "public"];
const TRAILING_SLASH_RE = /\/$/;

export type CliRunner = (
  command: string,
  args: string[],
  options: { cwd: string; env: NodeJS.ProcessEnv }
) => void;

export type UploadResult = {
  release: string;
  uploaded: string[];
  removed: string[];
};

export async function uploadSourcemaps(
  options: {
    cwd?: string;
    env?: ReleaseEnv & NodeJS.ProcessEnv;
    runner?: CliRunner;
  } = {}
): Promise<UploadResult> {
  const cwd = options.cwd ?? process.cwd();
  const env = options.env ?? process.env;
  const runner = options.runner ?? runSentryCLI;
  const release = resolveSentryRelease(env);
  const sourcemaps = await findSourcemaps(cwd, BUILD_ROOTS);
  if (sourcemaps.length === 0) {
    throw new Error("No sourcemaps found in dist or .output");
  }

  for (const sourcemap of sourcemaps) {
    runner(
      "sentry-cli",
      [
        "releases",
        "files",
        release,
        "upload-sourcemaps",
        sourcemap,
        "--rewrite",
        "--validate",
      ],
      { cwd, env }
    );
  }

  const removed = await removePublicSourcemaps(cwd, sourcemaps, PUBLIC_ROOTS);
  return { release, uploaded: sourcemaps, removed };
}

export async function findSourcemaps(
  cwd: string,
  roots: string[]
): Promise<string[]> {
  const sourcemaps: string[] = [];
  for (const root of roots) {
    await collectSourcemaps(cwd, root, sourcemaps);
  }
  return sourcemaps.sort();
}

export async function removePublicSourcemaps(
  cwd: string,
  sourcemaps: string[],
  publicRoots: string[]
): Promise<string[]> {
  const removed: string[] = [];
  for (const sourcemap of sourcemaps) {
    if (!isUnderPublicRoot(sourcemap, publicRoots)) {
      continue;
    }
    await rm(path.join(cwd, sourcemap), { force: true });
    removed.push(sourcemap);
  }
  return removed;
}

function runSentryCLI(
  command: string,
  args: string[],
  options: { cwd: string; env: NodeJS.ProcessEnv }
) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed`);
  }
}

async function collectSourcemaps(
  cwd: string,
  relativeDir: string,
  out: string[]
) {
  const absoluteDir = path.join(cwd, relativeDir);
  try {
    const info = await stat(absoluteDir);
    if (!info.isDirectory()) {
      return;
    }
  } catch {
    return;
  }

  const entries = await readdir(absoluteDir, { withFileTypes: true });
  for (const entry of entries) {
    const child = path.join(relativeDir, entry.name);
    if (entry.isDirectory()) {
      await collectSourcemaps(cwd, child, out);
      continue;
    }
    if (entry.isFile() && entry.name.endsWith(".map")) {
      out.push(toPosixPath(child));
    }
  }
}

function isUnderPublicRoot(file: string, publicRoots: string[]): boolean {
  return publicRoots.some((root) => {
    const normalizedRoot = toPosixPath(root).replace(TRAILING_SLASH_RE, "");
    return file === normalizedRoot || file.startsWith(`${normalizedRoot}/`);
  });
}

function toPosixPath(value: string): string {
  return value.split(path.sep).join(path.posix.sep);
}

const invokedPath = process.argv[1] ? fileURLToPath(import.meta.url) : "";

if (process.argv[1] && path.resolve(process.argv[1]) === invokedPath) {
  uploadSourcemaps()
    .then((result) => {
      console.log(
        `Uploaded ${result.uploaded.length} sourcemap(s) for ${result.release}; removed ${result.removed.length} public map file(s).`
      );
    })
    .catch((err) => {
      console.error(err instanceof Error ? err.message : err);
      process.exitCode = 1;
    });
}
