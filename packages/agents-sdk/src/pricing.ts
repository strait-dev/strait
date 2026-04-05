import { StraitSDKError, UnknownPricingError } from "./errors";
import type {
  ModelPricing,
  NormalizedUsageReport,
  PricingCatalog,
  UsageReport,
} from "./types";

const defaultEntries = [
  {
    provider: "local",
    model: "local-agent",
    inputCostMicrousd: 10,
    outputCostMicrousd: 10,
    aliases: ["local"],
  },
  {
    provider: "openai",
    model: "gpt-4.1",
    inputCostMicrousd: 2,
    outputCostMicrousd: 8,
    aliases: ["gpt-4.1-2025-04-14"],
  },
  {
    provider: "openai",
    model: "gpt-4.1-mini",
    inputCostMicrousd: 0.4,
    outputCostMicrousd: 1.6,
  },
  {
    provider: "openai",
    model: "gpt-4o",
    inputCostMicrousd: 2.5,
    outputCostMicrousd: 10,
    aliases: ["gpt-4o-2024-11-20"],
  },
  {
    provider: "openai",
    model: "gpt-4o-mini",
    inputCostMicrousd: 0.15,
    outputCostMicrousd: 0.6,
  },
  {
    provider: "openai",
    model: "o1",
    inputCostMicrousd: 15,
    outputCostMicrousd: 60,
    aliases: ["o1-2025-04-16"],
  },
  {
    provider: "openai",
    model: "o3-mini",
    inputCostMicrousd: 1.1,
    outputCostMicrousd: 4.4,
    aliases: ["o3-mini-2025-04-16"],
  },
  {
    provider: "openai",
    model: "o3",
    inputCostMicrousd: 2,
    outputCostMicrousd: 8,
    aliases: ["o3-2025-04-16"],
  },
  {
    provider: "anthropic",
    model: "claude-sonnet-4-5",
    inputCostMicrousd: 3,
    outputCostMicrousd: 15,
    cacheReadCostMicrousd: 0.3,
    cacheWriteCostMicrousd: 3.75,
    aliases: ["claude-sonnet-4-5-20250929", "claude-sonnet-4-20250514"],
  },
  {
    provider: "anthropic",
    model: "claude-opus-4",
    inputCostMicrousd: 15,
    outputCostMicrousd: 75,
    cacheReadCostMicrousd: 1.5,
    cacheWriteCostMicrousd: 18.75,
    aliases: ["claude-opus-4-20250514"],
  },
  {
    provider: "anthropic",
    model: "claude-haiku-4-5",
    inputCostMicrousd: 0.8,
    outputCostMicrousd: 4,
    cacheReadCostMicrousd: 0.08,
    cacheWriteCostMicrousd: 1,
    aliases: ["claude-haiku-4-5-20251001"],
  },
  {
    provider: "google",
    model: "gemini-2.5-pro",
    inputCostMicrousd: 1.25,
    outputCostMicrousd: 10,
    aliases: ["gemini-2.5-pro-preview-05-06"],
  },
  {
    provider: "google",
    model: "gemini-2.5-flash",
    inputCostMicrousd: 0.3,
    outputCostMicrousd: 2.5,
    aliases: ["gemini-2.5-flash-preview-05-20"],
  },
] as const satisfies PricingCatalog;

function normalizeKeyPart(value: string, field: string): string {
  const normalized = value.trim().toLowerCase();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function assertNonNegativeNumber(value: number, field: string): void {
  if (!Number.isFinite(value) || value < 0) {
    throw new StraitSDKError(`${field} must be a non-negative number`);
  }
}

function cloneEntry(entry: ModelPricing): ModelPricing {
  return {
    provider: normalizeKeyPart(entry.provider, "provider"),
    model: normalizeKeyPart(entry.model, "model"),
    inputCostMicrousd: entry.inputCostMicrousd,
    outputCostMicrousd: entry.outputCostMicrousd,
    cacheReadCostMicrousd: entry.cacheReadCostMicrousd,
    cacheWriteCostMicrousd: entry.cacheWriteCostMicrousd,
    aliases: entry.aliases?.map((alias) => normalizeKeyPart(alias, "alias")),
  };
}

export function createPricingCatalog(entries: PricingCatalog): PricingCatalog {
  const seen = new Set<string>();
  return entries.map((entry) => {
    const normalized = cloneEntry(entry);
    assertNonNegativeNumber(normalized.inputCostMicrousd, "inputCostMicrousd");
    assertNonNegativeNumber(
      normalized.outputCostMicrousd,
      "outputCostMicrousd"
    );

    const keys = [normalized.model, ...(normalized.aliases ?? [])];
    for (const key of keys) {
      const pricingKey = `${normalized.provider}:${key}`;
      if (seen.has(pricingKey)) {
        throw new StraitSDKError(`duplicate pricing entry for ${pricingKey}`);
      }
      seen.add(pricingKey);
    }

    return normalized;
  });
}

export const defaultPricingCatalog = createPricingCatalog(defaultEntries);

export function lookupPricing(
  catalog: PricingCatalog,
  provider: string,
  model: string
): ModelPricing | null {
  const providerKey = normalizeKeyPart(provider, "provider");
  const modelKey = normalizeKeyPart(model, "model");

  for (const entry of catalog) {
    if (entry.provider !== providerKey) {
      continue;
    }
    if (entry.model === modelKey || entry.aliases?.includes(modelKey)) {
      return entry;
    }
  }

  return null;
}

export function getPricingOrThrow(
  catalog: PricingCatalog,
  provider: string,
  model: string
): ModelPricing {
  const entry = lookupPricing(catalog, provider, model);
  if (entry == null) {
    throw new UnknownPricingError(provider, model);
  }
  return entry;
}

export function estimateUsageCostMicrousd(
  usage: Pick<
    UsageReport,
    | "provider"
    | "model"
    | "promptTokens"
    | "completionTokens"
    | "promptTokenDetails"
  >,
  catalog: PricingCatalog = defaultPricingCatalog
): number | null {
  assertNonNegativeNumber(usage.promptTokens, "promptTokens");
  assertNonNegativeNumber(usage.completionTokens, "completionTokens");

  const entry = lookupPricing(catalog, usage.provider, usage.model);
  if (entry == null) {
    return null;
  }

  // Account for cache token discounts when available.
  const cacheReadTokens = usage.promptTokenDetails?.cacheReadTokens ?? 0;
  const cacheWriteTokens = usage.promptTokenDetails?.cacheWriteTokens ?? 0;
  const normalInputTokens = Math.max(
    0,
    usage.promptTokens - cacheReadTokens - cacheWriteTokens
  );

  const inputCost =
    normalInputTokens * entry.inputCostMicrousd +
    cacheReadTokens * (entry.cacheReadCostMicrousd ?? entry.inputCostMicrousd) +
    cacheWriteTokens *
      (entry.cacheWriteCostMicrousd ?? entry.inputCostMicrousd);

  return Math.round(
    inputCost + usage.completionTokens * entry.outputCostMicrousd
  );
}

export function normalizeUsageReport(
  usage: UsageReport,
  catalog: PricingCatalog = defaultPricingCatalog
): NormalizedUsageReport {
  const provider = normalizeKeyPart(usage.provider, "provider");
  const model = normalizeKeyPart(usage.model, "model");

  assertNonNegativeNumber(usage.promptTokens, "promptTokens");
  assertNonNegativeNumber(usage.completionTokens, "completionTokens");

  const totalTokens =
    usage.totalTokens ?? usage.promptTokens + usage.completionTokens;
  assertNonNegativeNumber(totalTokens, "totalTokens");

  const estimatedCost = estimateUsageCostMicrousd(
    {
      provider,
      model,
      promptTokens: usage.promptTokens,
      completionTokens: usage.completionTokens,
    },
    catalog
  );

  const costMicrousd = usage.costMicrousd ?? estimatedCost ?? 0;
  assertNonNegativeNumber(costMicrousd, "costMicrousd");

  return {
    provider,
    model,
    promptTokens: usage.promptTokens,
    completionTokens: usage.completionTokens,
    totalTokens,
    costMicrousd,
  };
}
