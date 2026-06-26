import fs from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const appDir = resolve(__dirname, "../..");
const repoRoot = resolve(appDir, "../..");
const envPaths = [
  resolve(repoRoot, ".env"),
  resolve(appDir, ".env"),
  resolve(appDir, ".dev.vars"),
];

/** Load local dotenv values for Playwright setup processes. */
export function loadE2EEnv() {
  for (const envPath of envPaths) {
    if (!fs.existsSync(envPath)) {
      continue;
    }

    for (const line of fs.readFileSync(envPath, "utf-8").split("\n")) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) {
        continue;
      }

      const [key, ...valueParts] = trimmed.split("=");
      if (process.env[key]) {
        continue;
      }
      process.env[key] = unquote(valueParts.join("="));
    }
  }
}

function unquote(value: string) {
  const trimmed = value.trim();
  if (
    (trimmed.startsWith('"') && trimmed.endsWith('"')) ||
    (trimmed.startsWith("'") && trimmed.endsWith("'"))
  ) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}
