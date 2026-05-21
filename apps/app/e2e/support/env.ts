import fs from "node:fs";
import { resolve } from "node:path";

const devVarsPath = resolve(".dev.vars");

/** Load local Infisical-exported env values for Playwright setup processes. */
export function loadE2EEnv() {
  if (!fs.existsSync(devVarsPath)) {
    return;
  }

  for (const line of fs.readFileSync(devVarsPath, "utf-8").split("\n")) {
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
