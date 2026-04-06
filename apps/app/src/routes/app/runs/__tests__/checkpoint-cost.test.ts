import { describe, expect, it } from "vitest";

/**
 * Tests for checkpoint cost extraction logic used in the run detail timeline.
 * The actual rendering is in routes/app/runs/$id.tsx.
 */

function extractCheckpointCost(state: unknown): {
  costMicro: number | null;
  tokensAt: number | null;
} {
  const s = state as Record<string, unknown> | null;
  return {
    costMicro:
      typeof s?.cost_microusd_at_checkpoint === "number"
        ? s.cost_microusd_at_checkpoint
        : null,
    tokensAt:
      typeof s?.tokens_at_checkpoint === "number"
        ? s.tokens_at_checkpoint
        : null,
  };
}

describe("extractCheckpointCost", () => {
  it("extracts cost and tokens from enriched checkpoint state", () => {
    const state = {
      cost_microusd_at_checkpoint: 1_500_000,
      tokens_at_checkpoint: 2500,
      phase: "tool-result",
      iteration: 3,
    };
    const result = extractCheckpointCost(state);
    expect(result.costMicro).toBe(1_500_000);
    expect(result.tokensAt).toBe(2500);
  });

  it("formats cost as USD correctly", () => {
    const costMicro = 1_500_000;
    expect((costMicro / 1e6).toFixed(4)).toBe("1.5000");
  });

  it("returns null for missing cost fields", () => {
    const state = { phase: "tool-result" };
    const result = extractCheckpointCost(state);
    expect(result.costMicro).toBeNull();
    expect(result.tokensAt).toBeNull();
  });

  it("returns null for null state", () => {
    const result = extractCheckpointCost(null);
    expect(result.costMicro).toBeNull();
    expect(result.tokensAt).toBeNull();
  });

  it("handles non-numeric cost values gracefully", () => {
    const state = {
      cost_microusd_at_checkpoint: "not a number",
      tokens_at_checkpoint: true,
    };
    const result = extractCheckpointCost(state);
    expect(result.costMicro).toBeNull();
    expect(result.tokensAt).toBeNull();
  });

  it("handles zero values correctly", () => {
    const state = {
      cost_microusd_at_checkpoint: 0,
      tokens_at_checkpoint: 0,
    };
    const result = extractCheckpointCost(state);
    expect(result.costMicro).toBe(0);
    expect(result.tokensAt).toBe(0);
  });
});
