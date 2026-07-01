import { describe, expect, it } from "bun:test";
import {
  resolveTransactionalEmailTemplate,
  transactionalEmailTemplateIds,
} from "./registry";

const requiredTemplateIds = [
  "billing.contract_expired",
  "billing.dispute_alert",
  "billing.downgrade_http_jobs_warning",
  "billing.dunning_step",
  "billing.enterprise_contract_reminder",
  "billing.enterprise_welcome",
  "billing.invoice_upcoming",
  "billing.overage_alert",
  "billing.paid_plan_welcome",
  "billing.payment_failed",
  "billing.plan_changed",
  "billing.spending_limit_warning",
  "billing.trial_ending_soon",
  "billing.usage_report",
  "notification.budget_threshold",
  "notification.cost_anomaly",
  "notification.generic",
  "notification.spending_limit_reached",
  "notification.spending_limit_warning",
  "notification.usage_forecast_warning",
];

describe("transactional email registry", () => {
  it("contains every Go-owned email template", () => {
    const actual: string[] = [...transactionalEmailTemplateIds].sort();
    const expected = [...requiredTemplateIds].sort();
    expect(actual).toEqual(expected);
  });

  it("generates subjects from props", () => {
    expect(
      resolveTransactionalEmailTemplate("billing.payment_failed")?.subject({
        planName: "Pro",
      })
    ).toBe("Action required: payment failed");

    expect(
      resolveTransactionalEmailTemplate("billing.usage_report")?.subject({
        periodEnd: "Apr 30, 2026",
        periodStart: "Apr 1",
      })
    ).toBe("Your Strait usage report: Apr 1 to Apr 30, 2026");

    expect(
      resolveTransactionalEmailTemplate(
        "billing.enterprise_contract_reminder"
      )?.subject({
        autoRenew: false,
        daysRemaining: 7,
      })
    ).toBe("Enterprise contract expiring in 7 days");
  });

  it("renders usage report and notification templates from Go-shaped props", () => {
    expect(
      resolveTransactionalEmailTemplate("billing.usage_report")?.render({
        addonCount: 1,
        orgId: "org-1",
        overageAmount: "$1.00",
        periodEnd: "Apr 30, 2026",
        periodStart: "Apr 1",
        planTier: "pro",
      })
    ).toBeTruthy();

    expect(
      resolveTransactionalEmailTemplate(
        "notification.usage_forecast_warning"
      )?.render({
        daysUntilLimit: 2,
        orgId: "org-1",
        projectedRuns: 1_200_000,
        recommendedPlan: "scale",
      })
    ).toBeTruthy();
  });
});
