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
    provider: "anthropic",
    model: "claude-sonnet-4-5",
    inputCostMicrousd: 3,
    outputCostMicrousd: 15,
    aliases: ["claude-sonnet-4-5-20250929", "claude-sonnet-4-20250514"],
  },
  {
    provider: "anthropic",
    model: "claude-opus-4",
    inputCostMicrousd: 15,
    outputCostMicrousd: 75,
    aliases: ["claude-opus-4-20250514"],
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
    "provider" | "model" | "promptTokens" | "completionTokens"
  >,
  catalog: PricingCatalog = defaultPricingCatalog
): number | null {
  assertNonNegativeNumber(usage.promptTokens, "promptTokens");
  assertNonNegativeNumber(usage.completionTokens, "completionTokens");

  const entry = lookupPricing(catalog, usage.provider, usage.model);
  if (entry == null) {
    return null;
  }

  return Math.round(
    usage.promptTokens * entry.inputCostMicrousd +
      usage.completionTokens * entry.outputCostMicrousd
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
