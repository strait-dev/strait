import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const schemaPath = join(process.cwd(), "src/lib/api/schema.d.ts");

const removedPublicApiMarkers = [
  "/v1/runs/{runID}/usage",
  "/sdk/v1/runs/{runID}/usage",
  "/v1/runs/{runID}/tool-calls",
  "/sdk/v1/runs/{runID}/tool-call",
  "/sdk/v1/runs/{runID}/iteration",
  "SDKUsageRequest",
  "RunUsage",
  "list-run-usage",
  "sdk-usage",
  "list-run-tool-calls",
  "sdk-tool-call",
  "sdk-iteration",
  "tool_calls",
  "input_tokens",
  "output_tokens",
  "prompt_tokens",
  "completion_tokens",
  "ai_cost_cents",
  "ai_cost_microusd",
  "total_ai_cost_microusd",
  "by_model",
  "has_sso",
  "has_scim",
  "has_ip_allowlisting",
  "has_static_ips",
  "has_vpc_peering",
  "has_data_residency",
  "has_dedicated_compute",
  "has_priority_queue",
];

describe("generated API schema policy", () => {
  it("does not expose removed billing or AI-shaped launch surfaces", () => {
    const source = readFileSync(schemaPath, "utf8");

    for (const marker of removedPublicApiMarkers) {
      expect(source).not.toContain(marker);
    }
  });
});
