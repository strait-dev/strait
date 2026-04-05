import { describe, expect, it } from "vitest";

import {
  createPricingCatalog,
  defaultPricingCatalog,
  estimateUsageCostMicrousd,
  getPricingOrThrow,
  lookupPricing,
  normalizeUsageReport,
} from "./pricing";

const duplicatePricingEntryMessage = "duplicate pricing entry";

describe("pricing", () => {
  it("looks up entries case-insensitively across aliases", () => {
    const catalog = createPricingCatalog([
      {
        provider: "OpenAI",
        model: "gpt-4.1",
        aliases: ["gpt-4.1-latest"],
        inputCostMicrousd: 3,
        outputCostMicrousd: 12,
      },
    ]);

    const entry = getPricingOrThrow(catalog, "openai", "GPT-4.1-LATEST");

    expect(entry.model).toBe("gpt-4.1");
    expect(lookupPricing(catalog, "openai", "missing-model")).toBeNull();
  });

  it("estimates usage cost using per-token pricing", () => {
    const catalog = createPricingCatalog([
      {
        provider: "anthropic",
        model: "claude-sonnet",
        inputCostMicrousd: 4,
        outputCostMicrousd: 15,
      },
    ]);

    const cost = estimateUsageCostMicrousd(
      {
        provider: "Anthropic",
        model: "claude-sonnet",
        promptTokens: 7,
        completionTokens: 3,
      },
      catalog
    );

    expect(cost).toBe(73);
  });

  it("supports fractional per-token pricing for google models", () => {
    const cost = estimateUsageCostMicrousd(
      {
        provider: "google",
        model: "gemini-2.5-pro",
        promptTokens: 4,
        completionTokens: 2,
      },
      defaultPricingCatalog
    );

    expect(cost).toBe(25);
  });

  it("normalizes totals and cost when the caller omits them", () => {
    const catalog = createPricingCatalog([
      {
        provider: "openai",
        model: "gpt-4.1-mini",
        inputCostMicrousd: 2,
        outputCostMicrousd: 8,
      },
    ]);

    expect(
      normalizeUsageReport(
        {
          provider: "OpenAI",
          model: "gpt-4.1-mini",
          promptTokens: 10,
          completionTokens: 4,
        },
        catalog
      )
    ).toEqual({
      provider: "openai",
      model: "gpt-4.1-mini",
      promptTokens: 10,
      completionTokens: 4,
      totalTokens: 14,
      costMicrousd: 52,
    });
  });

  it("rejects duplicate provider/model aliases", () => {
    expect(() =>
      createPricingCatalog([
        {
          provider: "openai",
          model: "gpt-4.1",
          aliases: ["latest"],
          inputCostMicrousd: 1,
          outputCostMicrousd: 2,
        },
        {
          provider: "openai",
          model: "gpt-4.1-mini",
          aliases: ["LATEST"],
          inputCostMicrousd: 1,
          outputCostMicrousd: 2,
        },
      ])
    ).toThrow(duplicatePricingEntryMessage);
  });

  it("keeps randomized totals internally consistent", () => {
    const catalog = createPricingCatalog([
      {
        provider: "local",
        model: "local-agent",
        inputCostMicrousd: 1,
        outputCostMicrousd: 1,
      },
    ]);

    for (let i = 0; i < 100; i += 1) {
      const promptTokens = Math.floor(Math.random() * 100);
      const completionTokens = Math.floor(Math.random() * 100);
      const report = normalizeUsageReport(
        {
          provider: "local",
          model: "local-agent",
          promptTokens,
          completionTokens,
        },
        catalog
      );

      expect(report.totalTokens).toBe(promptTokens + completionTokens);
      expect(report.costMicrousd).toBe(promptTokens + completionTokens);
    }
  });

  it("ships default entries for openai, anthropic, and google", () => {
    expect(
      lookupPricing(defaultPricingCatalog, "openai", "gpt-4o")
    ).toBeTruthy();
    expect(
      lookupPricing(defaultPricingCatalog, "anthropic", "claude-sonnet-4-5")
    ).toBeTruthy();
    expect(
      lookupPricing(defaultPricingCatalog, "google", "gemini-2.5-pro")
    ).toBeTruthy();
  });

  it("includes O1/O3 reasoning models in default catalog", () => {
    const o1 = getPricingOrThrow(defaultPricingCatalog, "openai", "o1");
    expect(o1.inputCostMicrousd).toBe(15);
    expect(o1.outputCostMicrousd).toBe(60);

    const o3mini = getPricingOrThrow(
      defaultPricingCatalog,
      "openai",
      "o3-mini"
    );
    expect(o3mini.inputCostMicrousd).toBe(1.1);

    const o3 = getPricingOrThrow(defaultPricingCatalog, "openai", "o3");
    expect(o3.inputCostMicrousd).toBe(2);
    expect(o3.outputCostMicrousd).toBe(8);
  });

  it("resolves O1 alias", () => {
    const o1 = getPricingOrThrow(
      defaultPricingCatalog,
      "openai",
      "o1-2025-04-16"
    );
    expect(o1.model).toBe("o1");
  });

  it("includes claude-haiku-4-5 in default catalog", () => {
    const haiku = getPricingOrThrow(
      defaultPricingCatalog,
      "anthropic",
      "claude-haiku-4-5"
    );
    expect(haiku.inputCostMicrousd).toBe(0.8);
    expect(haiku.outputCostMicrousd).toBe(4);
  });

  it("estimates cost with cache token discounts", () => {
    // Claude Sonnet 4.5: input=3, cacheRead=0.3
    // 500 normal prompt tokens + 500 cache read tokens = 500*3 + 500*0.3 = 1650
    // 200 completion tokens at 15 = 3000
    // Total = 4650
    const cost = estimateUsageCostMicrousd({
      provider: "anthropic",
      model: "claude-sonnet-4-5",
      promptTokens: 1000,
      completionTokens: 200,
      promptTokenDetails: {
        cacheReadTokens: 500,
      },
    });
    expect(cost).toBe(Math.round(500 * 3 + 500 * 0.3 + 200 * 15));
  });

  it("cache discount does not apply when no cache tokens", () => {
    const withCache = estimateUsageCostMicrousd({
      provider: "anthropic",
      model: "claude-sonnet-4-5",
      promptTokens: 1000,
      completionTokens: 100,
    });
    // All 1000 tokens at full input rate (3) + 100 * 15 = 4500
    expect(withCache).toBe(Math.round(1000 * 3 + 100 * 15));
  });

  it("cache write tokens charged at premium rate", () => {
    // 500 normal + 500 cache write at 3.75 = 500*3 + 500*3.75 = 3375
    const cost = estimateUsageCostMicrousd({
      provider: "anthropic",
      model: "claude-sonnet-4-5",
      promptTokens: 1000,
      completionTokens: 0,
      promptTokenDetails: {
        cacheWriteTokens: 500,
      },
    });
    expect(cost).toBe(Math.round(500 * 3 + 500 * 3.75));
  });

  it("models without cache pricing use input rate for cache tokens", () => {
    // GPT-4o has no cache pricing fields.
    const cost = estimateUsageCostMicrousd({
      provider: "openai",
      model: "gpt-4o",
      promptTokens: 1000,
      completionTokens: 0,
      promptTokenDetails: {
        cacheReadTokens: 500,
      },
    });
    // Falls back to inputCostMicrousd for cache tokens: 1000 * 2.5 = 2500
    expect(cost).toBe(Math.round(1000 * 2.5));
  });
});
