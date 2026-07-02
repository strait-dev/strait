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
] as const;

type RequiredTemplateId = (typeof requiredTemplateIds)[number];

const samplePropsByTemplate = {
  "billing.contract_expired": {
    contractEndDate: "December 31, 2026",
  },
  "billing.dispute_alert": {
    disputeAmount: "$25.00",
  },
  "billing.downgrade_http_jobs_warning": {
    jobCount: 3,
    periodEnd: "April 30, 2026",
  },
  "billing.dunning_step": {
    planName: "Pro",
    step: 2,
  },
  "billing.enterprise_contract_reminder": {
    autoRenew: false,
    contractEndDate: "December 31, 2026",
    daysRemaining: 7,
  },
  "billing.enterprise_welcome": {},
  "billing.invoice_upcoming": {
    amountDue: "$125.00",
    dueDate: "May 1, 2026",
  },
  "billing.overage_alert": {
    includedAllowance: "$10.00",
    name: "",
    overageAmount: "$4.50",
    planName: "Pro",
  },
  "billing.paid_plan_welcome": {
    monthlyRunAllowance: "1000000",
    name: "",
    planName: "Pro",
  },
  "billing.payment_failed": {
    gracePeriodEnd: "April 15, 2026",
    name: "",
    planName: "Pro",
  },
  "billing.plan_changed": {
    effectiveDate: "April 15, 2026",
    name: "",
    newPlan: "Scale",
    previousPlan: "Pro",
  },
  "billing.spending_limit_warning": {
    currentSpend: "$80.00",
    name: "",
    percentUsed: "80%",
    planName: "Pro",
    spendingLimit: "$100.00",
  },
  "billing.trial_ending_soon": {
    daysRemaining: 3,
    trialEndDate: "April 15, 2026",
  },
  "billing.usage_report": {
    addonCount: 1,
    orgId: "org-1",
    overageAmount: "$1.00",
    periodEnd: "Apr 30, 2026",
    periodStart: "Apr 1",
    planTier: "pro",
  },
  "notification.budget_threshold": {
    budgetLimit: "1000000 micro-USD",
    dailyCost: "85000 micro-USD",
    projectId: "project-1",
    thresholdPercent: "80%",
  },
  "notification.cost_anomaly": {
    orgId: "org-1",
    sevenDayAverage: "15000 micro-USD",
    severity: "high",
    spikeRatio: "3.5x",
    todaySpend: "50000 micro-USD",
    topContributor: "job-x",
  },
  "notification.generic": {
    eventType: "unknown.event",
    payload: '{"field":"value"}',
  },
  "notification.spending_limit_reached": {
    currentSpend: "$100.00",
    orgId: "org-1",
    spendingLimit: "$100.00",
  },
  "notification.spending_limit_warning": {
    currentSpend: "$80.00",
    orgId: "org-1",
    overagePercent: "80%",
    spendingLimit: "$100.00",
  },
  "notification.usage_forecast_warning": {
    daysUntilLimit: 2,
    orgId: "org-1",
    projectedRuns: 1_200_000,
    recommendedPlan: "scale",
  },
} satisfies Record<RequiredTemplateId, Record<string, unknown>>;

describe("transactional email registry", () => {
  it("contains every Go-owned email template", () => {
    const actual: string[] = [...transactionalEmailTemplateIds].sort();
    const expected = [...requiredTemplateIds].sort();
    expect(actual).toEqual(expected);
  });

  it("defines schemas, renderers, and subjects for every Go-owned template", () => {
    for (const id of requiredTemplateIds) {
      const template = resolveTransactionalEmailTemplate(id);
      expect(template?.schema).toBeDefined();
      expect(template?.render).toBeTypeOf("function");
      expect(template?.subject).toBeTypeOf("function");
    }
  });

  it("accepts Go-shaped props for every template", () => {
    for (const id of requiredTemplateIds) {
      const template = resolveTransactionalEmailTemplate(id);
      const parsed = template?.schema.safeParse(samplePropsByTemplate[id]);
      expect(parsed?.success).toBe(true);
      if (parsed?.success) {
        expect(template?.subject(parsed.data)).toBeTruthy();
        expect(template?.render(parsed.data)).toBeTruthy();
      }
    }
  });

  it("rejects missing, wrongly typed, and unexpected props", () => {
    for (const id of requiredTemplateIds) {
      const template = resolveTransactionalEmailTemplate(id);
      expect(
        template?.schema.safeParse({
          ...samplePropsByTemplate[id],
          unexpected: true,
        }).success
      ).toBe(false);

      const firstKey = Object.keys(samplePropsByTemplate[id])[0];
      if (!firstKey) {
        continue;
      }

      expect(template?.schema.safeParse({}).success).toBe(false);

      const wrongTypeProps = {
        ...samplePropsByTemplate[id],
        [firstKey]: { invalid: true },
      };
      expect(template?.schema.safeParse(wrongTypeProps).success).toBe(false);
    }
  });

  it("generates subjects from props", () => {
    const paymentFailed = resolveTransactionalEmailTemplate(
      "billing.payment_failed"
    );
    expect(
      paymentFailed?.subject(
        paymentFailed.schema.parse(
          samplePropsByTemplate["billing.payment_failed"]
        )
      )
    ).toBe("Action required: payment failed");

    const usageReport = resolveTransactionalEmailTemplate(
      "billing.usage_report"
    );
    expect(
      usageReport?.subject(
        usageReport.schema.parse(samplePropsByTemplate["billing.usage_report"])
      )
    ).toBe("Your Strait usage report: Apr 1 to Apr 30, 2026");

    const reminder = resolveTransactionalEmailTemplate(
      "billing.enterprise_contract_reminder"
    );
    expect(
      reminder?.subject(
        reminder.schema.parse(
          samplePropsByTemplate["billing.enterprise_contract_reminder"]
        )
      )
    ).toBe("Enterprise contract expiring in 7 days");
  });

  it("renders usage report and notification templates from Go-shaped props", () => {
    const usageReport = resolveTransactionalEmailTemplate(
      "billing.usage_report"
    );
    expect(
      usageReport?.render(
        usageReport.schema.parse(samplePropsByTemplate["billing.usage_report"])
      )
    ).toBeTruthy();

    const forecast = resolveTransactionalEmailTemplate(
      "notification.usage_forecast_warning"
    );
    expect(
      forecast?.render(
        forecast.schema.parse(
          samplePropsByTemplate["notification.usage_forecast_warning"]
        )
      )
    ).toBeTruthy();
  });
});
