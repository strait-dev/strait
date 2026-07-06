import { createElement, type ReactElement } from "react";
import { z } from "zod";
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

export type TransactionalTemplate<TProps extends z.ZodType = z.ZodType> = {
  schema: TProps;
  render: (props: z.infer<TProps>) => ReactElement;
  subject: (props: z.infer<TProps>) => string;
};

const defineTemplate = <TProps extends z.ZodType>(
  template: TransactionalTemplate<TProps>
): TransactionalTemplate<TProps> => template;

const strictObject = <TShape extends z.ZodRawShape>(shape: TShape) =>
  z.strictObject(shape);

const requiredString = () => z.string().min(1);
// Zod 4's z.number() rejects Infinity, -Infinity, and NaN by default, so the
// deprecated .finite() check is redundant; the name documents the requirement.
const finiteNumber = () => z.number();
const noPropsSchema = z.strictObject({});
const displayValueSchema = z.union([z.string().min(1), z.number()]);

const displayValue = (value: string | number): string =>
  typeof value === "number" ? value.toLocaleString("en-US") : value;

export const transactionalEmailTemplates = {
  "billing.contract_expired": defineTemplate({
    schema: strictObject({
      contractEndDate: requiredString(),
    }),
    render: (props) =>
      createElement(ContractExpired, {
        contractEndDate: props.contractEndDate,
      }),
    subject: () => "Your enterprise contract has expired",
  }),
  "billing.dispute_alert": defineTemplate({
    schema: strictObject({
      disputeAmount: requiredString(),
    }),
    render: (props) =>
      createElement(DisputeAlert, {
        disputeAmount: props.disputeAmount,
      }),
    subject: () => "Payment dispute received",
  }),
  "billing.downgrade_http_jobs_warning": defineTemplate({
    schema: strictObject({
      jobCount: finiteNumber(),
      periodEnd: requiredString(),
    }),
    render: (props) =>
      createElement(DowngradeHTTPJobsWarning, {
        jobCount: props.jobCount,
        periodEnd: props.periodEnd,
      }),
    subject: (props) =>
      `Your ${props.jobCount} HTTP-mode jobs will be paused on ${props.periodEnd}`,
  }),
  "billing.dunning_step": defineTemplate({
    schema: strictObject({
      planName: requiredString(),
      step: finiteNumber(),
    }),
    render: (props) =>
      createElement(DunningStep, {
        planName: props.planName,
        step: props.step,
      }),
    subject: (props) => {
      switch (props.step) {
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
  }),
  "billing.enterprise_contract_reminder": defineTemplate({
    schema: strictObject({
      autoRenew: z.boolean(),
      contractEndDate: requiredString(),
      daysRemaining: finiteNumber(),
    }),
    render: (props) =>
      createElement(EnterpriseContractReminder, {
        autoRenew: props.autoRenew,
        contractEndDate: props.contractEndDate,
        daysRemaining: props.daysRemaining,
      }),
    subject: (props) =>
      props.autoRenew
        ? `Enterprise contract renewing in ${props.daysRemaining} days`
        : `Enterprise contract expiring in ${props.daysRemaining} days`,
  }),
  "billing.enterprise_welcome": defineTemplate({
    schema: noPropsSchema,
    render: () => createElement(EnterpriseWelcome),
    subject: () => "Welcome to Strait Enterprise!",
  }),
  "billing.invoice_upcoming": defineTemplate({
    schema: strictObject({
      amountDue: requiredString(),
      dueDate: requiredString(),
    }),
    render: (props) =>
      createElement(InvoiceUpcoming, {
        amountDue: props.amountDue,
        dueDate: props.dueDate,
      }),
    subject: () => "Upcoming invoice",
  }),
  "billing.overage_alert": defineTemplate({
    schema: strictObject({
      includedAllowance: requiredString(),
      name: z.string(),
      overageAmount: requiredString(),
      planName: requiredString(),
    }),
    render: (props) =>
      createElement(OverageAlert, {
        includedAllowance: props.includedAllowance,
        name: props.name,
        overageAmount: props.overageAmount,
        planName: props.planName,
      }),
    subject: (props) => `Overage alert - ${props.planName} plan`,
  }),
  "billing.paid_plan_welcome": defineTemplate({
    schema: strictObject({
      monthlyRunAllowance: requiredString(),
      name: z.string(),
      planName: requiredString(),
    }),
    render: (props) =>
      createElement(PaidPlanWelcome, {
        monthlyRunAllowance: props.monthlyRunAllowance,
        name: props.name,
        planName: props.planName,
      }),
    subject: (props) => `Welcome to Strait ${props.planName}!`,
  }),
  "billing.payment_failed": defineTemplate({
    schema: strictObject({
      gracePeriodEnd: requiredString(),
      name: z.string(),
      planName: requiredString(),
    }),
    render: (props) =>
      createElement(PaymentFailed, {
        gracePeriodEnd: props.gracePeriodEnd,
        name: props.name,
        planName: props.planName,
      }),
    subject: () => "Action required: payment failed",
  }),
  "billing.plan_changed": defineTemplate({
    schema: strictObject({
      effectiveDate: requiredString(),
      name: z.string(),
      newPlan: requiredString(),
      previousPlan: requiredString(),
    }),
    render: (props) =>
      createElement(PlanChanged, {
        effectiveDate: props.effectiveDate,
        name: props.name,
        newPlan: props.newPlan,
        previousPlan: props.previousPlan,
      }),
    subject: (props) => `Plan changed to ${props.newPlan}`,
  }),
  "billing.spending_limit_warning": defineTemplate({
    schema: strictObject({
      currentSpend: requiredString(),
      name: z.string(),
      percentUsed: requiredString(),
      planName: requiredString(),
      spendingLimit: requiredString(),
    }),
    render: (props) =>
      createElement(SpendingLimitWarning, {
        currentSpend: props.currentSpend,
        name: props.name,
        percentUsed: props.percentUsed,
        planName: props.planName,
        spendingLimit: props.spendingLimit,
      }),
    subject: (props) => `Spending limit warning - ${props.percentUsed} used`,
  }),
  "billing.trial_ending_soon": defineTemplate({
    schema: strictObject({
      daysRemaining: finiteNumber(),
      trialEndDate: requiredString(),
    }),
    render: (props) =>
      createElement(TrialEndingSoon, {
        daysRemaining: props.daysRemaining,
        trialEndDate: props.trialEndDate,
      }),
    subject: (props) => `Temporary access ends in ${props.daysRemaining} days`,
  }),
  "billing.usage_report": defineTemplate({
    schema: strictObject({
      addonCount: finiteNumber(),
      orgId: requiredString(),
      overageAmount: requiredString().optional(),
      periodEnd: requiredString(),
      periodStart: requiredString(),
      planTier: requiredString(),
    }),
    render: (props) =>
      createElement(UsageReport, {
        addonCount: props.addonCount,
        orgId: props.orgId,
        overageAmount: props.overageAmount,
        periodEnd: props.periodEnd,
        periodStart: props.periodStart,
        planTier: props.planTier,
      }),
    subject: (props) =>
      `Your Strait usage report: ${props.periodStart} to ${props.periodEnd}`,
  }),
  "notification.budget_threshold": defineTemplate({
    schema: strictObject({
      budgetLimit: requiredString(),
      dailyCost: requiredString(),
      projectId: requiredString(),
      thresholdPercent: requiredString(),
    }),
    render: (props) =>
      createElement(BudgetThreshold, {
        budgetLimit: props.budgetLimit,
        dailyCost: props.dailyCost,
        projectId: props.projectId,
        thresholdPercent: props.thresholdPercent,
      }),
    subject: () => "Compute budget threshold reached",
  }),
  "notification.cost_anomaly": defineTemplate({
    schema: strictObject({
      orgId: requiredString(),
      sevenDayAverage: requiredString(),
      severity: requiredString(),
      spikeRatio: requiredString(),
      todaySpend: requiredString(),
      topContributor: requiredString(),
    }),
    render: (props) =>
      createElement(CostAnomaly, {
        orgId: props.orgId,
        sevenDayAverage: props.sevenDayAverage,
        severity: props.severity,
        spikeRatio: props.spikeRatio,
        todaySpend: props.todaySpend,
        topContributor: props.topContributor,
      }),
    subject: () => "Cost anomaly detected - unusual spending spike",
  }),
  "notification.generic": defineTemplate({
    schema: strictObject({
      eventType: requiredString(),
      payload: requiredString(),
    }),
    render: (props) =>
      createElement(GenericNotification, {
        eventType: props.eventType,
        payload: props.payload,
      }),
    subject: (props) => `Strait notification: ${props.eventType}`,
  }),
  "notification.spending_limit_reached": defineTemplate({
    schema: strictObject({
      currentSpend: requiredString(),
      orgId: requiredString(),
      spendingLimit: requiredString(),
    }),
    render: (props) =>
      createElement(NotificationSpendingLimitReached, {
        currentSpend: props.currentSpend,
        orgId: props.orgId,
        spendingLimit: props.spendingLimit,
      }),
    subject: () => "Spending limit reached - 100%",
  }),
  "notification.spending_limit_warning": defineTemplate({
    schema: strictObject({
      currentSpend: requiredString(),
      orgId: requiredString(),
      overagePercent: requiredString(),
      spendingLimit: requiredString(),
    }),
    render: (props) =>
      createElement(NotificationSpendingLimitWarning, {
        currentSpend: props.currentSpend,
        orgId: props.orgId,
        overagePercent: props.overagePercent,
        spendingLimit: props.spendingLimit,
      }),
    subject: () => "Spending limit warning - 80% reached",
  }),
  "notification.usage_forecast_warning": defineTemplate({
    schema: strictObject({
      daysUntilLimit: finiteNumber(),
      orgId: requiredString(),
      projectedRuns: displayValueSchema,
      recommendedPlan: requiredString(),
    }),
    render: (props) =>
      createElement(UsageForecastWarning, {
        daysUntilLimit: props.daysUntilLimit,
        orgId: props.orgId,
        projectedRuns: displayValue(props.projectedRuns),
        recommendedPlan: props.recommendedPlan,
      }),
    subject: () => "Usage forecast warning",
  }),
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
