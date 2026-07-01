import { createElement, type ReactElement } from "react";
import ContractExpired from "./billing/contract-expired";
import DisputeAlert from "./billing/dispute-alert";
import DowngradeHTTPJobsWarning from "./billing/downgrade-http-jobs-warning";
import DunningStep from "./billing/dunning-step";
import EnterpriseContractReminder from "./billing/enterprise-contract-reminder";
import EnterpriseWelcome from "./billing/enterprise-welcome";
import InvoiceUpcoming from "./billing/invoice-upcoming";
import OverageAlert from "./billing/overage-alert";
import PaidPlanWelcome from "./billing/paid-plan-welcome";
import PaymentFailed from "./billing/payment-failed";
import PlanChanged from "./billing/plan-changed";
import SpendingLimitWarning from "./billing/spending-limit-warning";
import TrialEndingSoon from "./billing/trial-ending-soon";
import UsageReport from "./billing/usage-report";
import BudgetThreshold from "./notification/budget-threshold";
import CostAnomaly from "./notification/cost-anomaly";
import GenericNotification from "./notification/generic-notification";
import NotificationSpendingLimitReached from "./notification/spending-limit-reached";
import NotificationSpendingLimitWarning from "./notification/spending-limit-warning";
import UsageForecastWarning from "./notification/usage-forecast-warning";

type TemplateProps = Record<string, unknown>;

type TransactionalTemplate = {
  render: (props: TemplateProps) => ReactElement;
  subject: (props: TemplateProps) => string;
};

const stringProp = (
  props: TemplateProps,
  key: string,
  fallback = ""
): string => {
  const value = props[key];
  return typeof value === "string" ? value : fallback;
};

const numberProp = (
  props: TemplateProps,
  key: string,
  fallback = 0
): number => {
  const value = props[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
};

const displayProp = (
  props: TemplateProps,
  key: string,
  fallback = ""
): string => {
  const value = props[key];
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return value.toLocaleString("en-US");
  }
  return fallback;
};

const booleanProp = (
  props: TemplateProps,
  key: string,
  fallback = false
): boolean => {
  const value = props[key];
  return typeof value === "boolean" ? value : fallback;
};

export const transactionalEmailTemplates = {
  "billing.contract_expired": {
    render: (props) =>
      createElement(ContractExpired, {
        contractEndDate: stringProp(props, "contractEndDate"),
      }),
    subject: () => "Your enterprise contract has expired",
  },
  "billing.dispute_alert": {
    render: (props) =>
      createElement(DisputeAlert, {
        disputeAmount: stringProp(props, "disputeAmount"),
      }),
    subject: () => "Payment dispute received",
  },
  "billing.downgrade_http_jobs_warning": {
    render: (props) =>
      createElement(DowngradeHTTPJobsWarning, {
        jobCount: numberProp(props, "jobCount"),
        periodEnd: stringProp(props, "periodEnd"),
      }),
    subject: (props) =>
      `Your ${numberProp(props, "jobCount")} HTTP-mode jobs will be paused on ${stringProp(props, "periodEnd")}`,
  },
  "billing.dunning_step": {
    render: (props) =>
      createElement(DunningStep, {
        planName: stringProp(props, "planName"),
        step: numberProp(props, "step"),
      }),
    subject: (props) => {
      switch (numberProp(props, "step")) {
        case 1:
          return "Payment failed - action required";
        case 2:
          return "Payment still past due (day 3)";
        case 3:
          return "Payment still past due (day 7)";
        case 4:
          return "Access restricted - payment required";
        case 5:
          return "Final notice before suspension";
        case 6:
          return "Subscription suspended";
        default:
          return "Payment past due";
      }
    },
  },
  "billing.enterprise_contract_reminder": {
    render: (props) =>
      createElement(EnterpriseContractReminder, {
        autoRenew: booleanProp(props, "autoRenew"),
        contractEndDate: stringProp(props, "contractEndDate"),
        daysRemaining: numberProp(props, "daysRemaining"),
      }),
    subject: (props) =>
      booleanProp(props, "autoRenew")
        ? `Enterprise contract renewing in ${numberProp(props, "daysRemaining")} days`
        : `Enterprise contract expiring in ${numberProp(props, "daysRemaining")} days`,
  },
  "billing.enterprise_welcome": {
    render: () => createElement(EnterpriseWelcome),
    subject: () => "Welcome to Strait Enterprise!",
  },
  "billing.invoice_upcoming": {
    render: (props) =>
      createElement(InvoiceUpcoming, {
        amountDue: stringProp(props, "amountDue"),
        dueDate: stringProp(props, "dueDate"),
      }),
    subject: () => "Upcoming invoice",
  },
  "billing.overage_alert": {
    render: (props) =>
      createElement(OverageAlert, {
        includedAllowance: stringProp(props, "includedAllowance"),
        name: stringProp(props, "name"),
        overageAmount: stringProp(props, "overageAmount"),
        planName: stringProp(props, "planName"),
      }),
    subject: (props) => `Overage alert - ${stringProp(props, "planName")} plan`,
  },
  "billing.paid_plan_welcome": {
    render: (props) =>
      createElement(PaidPlanWelcome, {
        monthlyRunAllowance: stringProp(props, "monthlyRunAllowance"),
        name: stringProp(props, "name"),
        planName: stringProp(props, "planName"),
      }),
    subject: (props) => `Welcome to Strait ${stringProp(props, "planName")}!`,
  },
  "billing.payment_failed": {
    render: (props) =>
      createElement(PaymentFailed, {
        gracePeriodEnd: stringProp(props, "gracePeriodEnd"),
        name: stringProp(props, "name"),
        planName: stringProp(props, "planName"),
      }),
    subject: () => "Action required: payment failed",
  },
  "billing.plan_changed": {
    render: (props) =>
      createElement(PlanChanged, {
        effectiveDate: stringProp(props, "effectiveDate"),
        name: stringProp(props, "name"),
        newPlan: stringProp(props, "newPlan"),
        previousPlan: stringProp(props, "previousPlan"),
      }),
    subject: (props) => `Plan changed to ${stringProp(props, "newPlan")}`,
  },
  "billing.spending_limit_warning": {
    render: (props) =>
      createElement(SpendingLimitWarning, {
        currentSpend: stringProp(props, "currentSpend"),
        name: stringProp(props, "name"),
        percentUsed: stringProp(props, "percentUsed"),
        planName: stringProp(props, "planName"),
        spendingLimit: stringProp(props, "spendingLimit"),
      }),
    subject: (props) =>
      `Spending limit warning - ${stringProp(props, "percentUsed")} used`,
  },
  "billing.trial_ending_soon": {
    render: (props) =>
      createElement(TrialEndingSoon, {
        daysRemaining: numberProp(props, "daysRemaining"),
        trialEndDate: stringProp(props, "trialEndDate"),
      }),
    subject: (props) =>
      `Temporary access ends in ${numberProp(props, "daysRemaining")} days`,
  },
  "billing.usage_report": {
    render: (props) =>
      createElement(UsageReport, {
        addonCount: numberProp(props, "addonCount"),
        orgId: stringProp(props, "orgId"),
        overageAmount: stringProp(props, "overageAmount") || undefined,
        periodEnd: stringProp(props, "periodEnd"),
        periodStart: stringProp(props, "periodStart"),
        planTier: stringProp(props, "planTier"),
      }),
    subject: (props) =>
      `Your Strait usage report: ${stringProp(props, "periodStart")} to ${stringProp(props, "periodEnd")}`,
  },
  "notification.budget_threshold": {
    render: (props) =>
      createElement(BudgetThreshold, {
        budgetLimit: stringProp(props, "budgetLimit"),
        dailyCost: stringProp(props, "dailyCost"),
        projectId: stringProp(props, "projectId"),
        thresholdPercent: stringProp(props, "thresholdPercent"),
      }),
    subject: () => "Compute budget threshold reached",
  },
  "notification.cost_anomaly": {
    render: (props) =>
      createElement(CostAnomaly, {
        orgId: stringProp(props, "orgId"),
        sevenDayAverage: stringProp(props, "sevenDayAverage"),
        severity: stringProp(props, "severity"),
        spikeRatio: stringProp(props, "spikeRatio"),
        todaySpend: stringProp(props, "todaySpend"),
        topContributor: stringProp(props, "topContributor"),
      }),
    subject: () => "Cost anomaly detected - unusual spending spike",
  },
  "notification.generic": {
    render: (props) =>
      createElement(GenericNotification, {
        eventType: stringProp(props, "eventType"),
        payload: stringProp(props, "payload"),
      }),
    subject: (props) =>
      `Strait notification: ${stringProp(props, "eventType")}`,
  },
  "notification.spending_limit_reached": {
    render: (props) =>
      createElement(NotificationSpendingLimitReached, {
        currentSpend: stringProp(props, "currentSpend"),
        orgId: stringProp(props, "orgId"),
        spendingLimit: stringProp(props, "spendingLimit"),
      }),
    subject: () => "Spending limit reached - 100%",
  },
  "notification.spending_limit_warning": {
    render: (props) =>
      createElement(NotificationSpendingLimitWarning, {
        currentSpend: stringProp(props, "currentSpend"),
        orgId: stringProp(props, "orgId"),
        overagePercent: stringProp(props, "overagePercent"),
        spendingLimit: stringProp(props, "spendingLimit"),
      }),
    subject: () => "Spending limit warning - 80% reached",
  },
  "notification.usage_forecast_warning": {
    render: (props) =>
      createElement(UsageForecastWarning, {
        daysUntilLimit: numberProp(props, "daysUntilLimit"),
        orgId: stringProp(props, "orgId"),
        projectedRuns: displayProp(props, "projectedRuns"),
        recommendedPlan: stringProp(props, "recommendedPlan"),
      }),
    subject: () => "Usage forecast warning",
  },
} satisfies Record<string, TransactionalTemplate>;

export type TransactionalEmailTemplateId =
  keyof typeof transactionalEmailTemplates;

export const transactionalEmailTemplateIds = Object.keys(
  transactionalEmailTemplates
) as TransactionalEmailTemplateId[];

export const resolveTransactionalEmailTemplate = (
  template: string
): TransactionalTemplate | undefined =>
  transactionalEmailTemplates[template as TransactionalEmailTemplateId];
