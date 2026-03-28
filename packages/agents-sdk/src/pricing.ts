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
] as const satisfies PricingCatalog;

function normalizeKeyPart(value: string, field: string): string {
  const normalized = value.trim().toLowerCase();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function assertNonNegativeInt(value: number, field: string): void {
  if (!Number.isInteger(value) || value < 0) {
    throw new StraitSDKError(`${field} must be a non-negative integer`);
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
    assertNonNegativeInt(normalized.inputCostMicrousd, "inputCostMicrousd");
    assertNonNegativeInt(normalized.outputCostMicrousd, "outputCostMicrousd");

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
  usage: Pick<UsageReport, "provider" | "model" | "promptTokens" | "completionTokens">,
  catalog: PricingCatalog = defaultPricingCatalog
): number | null {
  assertNonNegativeInt(usage.promptTokens, "promptTokens");
  assertNonNegativeInt(usage.completionTokens, "completionTokens");

  const entry = lookupPricing(catalog, usage.provider, usage.model);
  if (entry == null) {
    return null;
  }

  return (
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

  assertNonNegativeInt(usage.promptTokens, "promptTokens");
  assertNonNegativeInt(usage.completionTokens, "completionTokens");

  const totalTokens = usage.totalTokens ?? usage.promptTokens + usage.completionTokens;
  assertNonNegativeInt(totalTokens, "totalTokens");

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
  assertNonNegativeInt(costMicrousd, "costMicrousd");

  return {
    provider,
    model,
    promptTokens: usage.promptTokens,
    completionTokens: usage.completionTokens,
    totalTokens,
    costMicrousd,
  };
}
