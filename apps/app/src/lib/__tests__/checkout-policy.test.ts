import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const sourceRoot = join(process.cwd(), "src");

describe("checkout policy", () => {
  it("does not enable Stripe promotion codes for launch checkout", () => {
    const checkoutFiles = [
      "routes/app/upgrade.tsx",
      "components/billing/addons-tab.tsx",
    ];

    for (const file of checkoutFiles) {
      const source = readFileSync(join(sourceRoot, file), "utf8");
      expect(source).not.toContain("allow_promotion_codes: true");
    }
  });

  it("keeps Enterprise on contact-sales instead of self-serve checkout", () => {
    const source = readFileSync(
      join(sourceRoot, "routes/app/upgrade.tsx"),
      "utf8"
    );

    expect(source).toContain('targetPlan === "enterprise"');
    expect(source).toContain(
      'window.location.assign("/app/enterprise-contact")'
    );
    expect(source).not.toContain("STRIPE_ENTERPRISE_STARTER_YEARLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_ENTERPRISE_GROWTH_YEARLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_ENTERPRISE_LARGE_YEARLY_PRICE_ID");
  });

  it("resolves plan checkout prices by shared Stripe lookup keys", () => {
    const source = readFileSync(
      join(sourceRoot, "routes/app/upgrade.tsx"),
      "utf8"
    );

    expect(source).toContain("PLAN_LOOKUP_KEYS");
    expect(source).toContain("stripe.prices.list");
    expect(source).toContain("lookup_keys: [lookupKey]");
    expect(source).not.toContain("STRIPE_STARTER_MONTHLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_STARTER_YEARLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_PRO_MONTHLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_PRO_YEARLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_SCALE_MONTHLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_SCALE_YEARLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_BUSINESS_MONTHLY_PRICE_ID");
    expect(source).not.toContain("STRIPE_BUSINESS_YEARLY_PRICE_ID");
  });

  it("keeps roadmap add-ons out of app checkout price maps", () => {
    const source = readFileSync(
      join(sourceRoot, "components/billing/addons-tab.tsx"),
      "utf8"
    );

    expect(source).toContain("getAddonCatalogItem(data.checkoutSlug)");
    expect(source).toContain("isAddonAvailableOnPlan(addon.type, usage.plan)");
    expect(source).toContain("stripe.prices.list");
    expect(source).toContain("lookup_keys: [addon.lookupKey]");
    expect(source).not.toContain("STRIPE_ADDON_CONCURRENCY_100_PRICE_ID");
    expect(source).not.toContain("STRIPE_ADDON_HISTORY_30D_PRICE_ID");
    expect(source).not.toContain("STRIPE_ADDON_ENVIRONMENTS_5_PRICE_ID");
    expect(source).not.toContain("STRIPE_ADDON_COMPLIANCE_ARCHIVE_PRICE_ID");
    expect(source).not.toContain("STRIPE_ADDON_DEDICATED_WORKER_POOL_PRICE_ID");
    expect(source).not.toContain("STRIPE_ADDON_DEDICATED_POOL_PRICE_ID");
    expect(source).not.toContain("compliance_archive:");
    expect(source).not.toContain("dedicated_workers:");
  });
});
