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
  "total_tokens",
  "max_tokens_per_run",
  "max_tool_calls_per_run",
  "max_iterations_per_run",
  "allowed_tools",
  "blocked_tools",
  "compute_credit_microusd",
  "compute_credit",
  "included_credit_microusd",
  "credit_used_percent",
  "credit_remaining_microusd",
  "projected_monthly_compute_usd",
  "compute_discount_pct",
  "compute_cost_microusd",
  "compute_microusd",
  "by_model",
  ["BY", "OK"].join(""),
  "OpenAI",
  "Anthropic",
  "LLM",
  "model_usage",
  "model usage",
  "has_sso",
  "has_scim",
  "has_ip_allowlisting",
  "has_static_ips",
  "has_vpc_peering",
  "has_data_residency",
  "has_dedicated_compute",
  "has_reserved_capacity",
  "has_priority_queue",
  "has_session_management",
  "has_secret_rotation",
  "has_siem_export",
  "preferred_regions",
  "default_region",
  "allowed_regions",
  "RegionResponse",
  "stream-deployment-logs",
  "machine_id",
];

const retiredModelCostMarkers = [
  ["ai", "cost", "cents"],
  ["ai", "cost", "microusd"],
  ["total", "ai", "cost", "microusd"],
].map((parts) => parts.join("_"));

describe("generated API schema policy", () => {
  it("does not expose removed billing or model-usage-shaped launch surfaces", () => {
    const source = readFileSync(schemaPath, "utf8");

    for (const marker of [
      ...removedPublicApiMarkers,
      ...retiredModelCostMarkers,
    ]) {
      expect(source).not.toContain(marker);
    }
  });
});
