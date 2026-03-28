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
});
