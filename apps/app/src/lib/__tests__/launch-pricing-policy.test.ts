import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { PLANS } from "@strait/billing";
import { describe, expect, it } from "vitest";

const appSrc = join(process.cwd(), "src");
const sourceFileRegex = /\.(ts|tsx)$/;
const policyTestPath = join(
  process.cwd(),
  "src/lib/__tests__/launch-pricing-policy.test.ts"
);
const apiSchemaPolicyTestPath = join(
  process.cwd(),
  "src/lib/__tests__/api-schema-policy.test.ts"
);

function readSourceFiles(dir: string): string {
  let source = "";
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      source += readSourceFiles(path);
      continue;
    }
    if (
      sourceFileRegex.test(entry) &&
      path !== policyTestPath &&
      path !== apiSchemaPolicyTestPath &&
      !path.endsWith("schema.d.ts")
    ) {
      source += `\n${readFileSync(path, "utf8")}`;
    }
  }
  return source;
}

describe("launch pricing app policy", () => {
  it("does not carry a compute-preset cost estimator", () => {
    for (const removed of [
      "src/components/billing/cost-estimate-card.tsx",
      "src/hooks/billing/use-cost-estimate.ts",
    ]) {
      expect(existsSync(join(process.cwd(), removed))).toBe(false);
    }
  });

  it("does not carry a private app pricing catalog", () => {
    expect(
      existsSync(join(process.cwd(), "src/lib/billing-constants.ts"))
    ).toBe(false);
  });

  it("keeps launch pricing copy tied to run overage, not compute or member overage", () => {
    const source = readSourceFiles(appSrc);
    for (const stale of [
      "Cost per Run",
      "Estimated cost",
      "Compute Cost",
      "Compute discount",
      "member overage",
      "includedCreditMonthly",
      "computeDiscountPct",
      "Trial ends tomorrow!",
      "Trial ending very soon!",
      "Trial ending soon!",
      "TrialUpgradeCard",
      "trial-upgrade-card",
      "getTrialContent",
      "trialMessage",
      "custom resources",
      "Roadmap security features via sales",
      "/v1/analytics/compute",
      "compute usage",
    ]) {
      expect(source).not.toContain(stale);
    }
  });

  it("does not expose self-serve trial metadata in the shared plan catalog", () => {
    for (const plan of Object.values(PLANS)) {
      expect(Object.hasOwn(plan, "trial")).toBe(false);
    }
  });

  it("keeps the webhook event picker aligned with launch billing events", () => {
    const source = readFileSync(
      join(appSrc, "routes/app/webhooks/new.tsx"),
      "utf8"
    );

    for (const eventType of [
      "billing.cap_warning",
      "billing.cap_reached",
      "billing.cap_disabled",
      "billing.overage_disabled",
      "billing.suspended",
      "billing.delinquent",
      "billing.payment_succeeded",
      "schedule.suspended",
      "workflow.registration_rejected",
      "sla.credit_issued",
    ]) {
      expect(source).toContain(`value: "${eventType}"`);
    }
    expect(source).not.toContain("compute_budget_warning");
  });
});
