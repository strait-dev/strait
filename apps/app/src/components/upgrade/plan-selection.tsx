import { HugeiconsIcon } from "@hugeicons/react";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import { cn } from "@strait/ui/utils/index";
import { useCallback } from "react";
import PricingCalculator from "@/components/upgrade/pricing-calculator";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { formatCurrency } from "@/lib/format";
import { CheckIcon, StarIcon } from "@/lib/icons";
import { PERCENTAGE_MULTIPLIER } from "@/utils/constants";

const MONTHS_IN_A_YEAR = 12;
const CENTS_TO_DOLLARS = 100;

import type { ComparisonFeature, PricingPlan } from "@/hooks/billing/use-plans";

type PlanType = "free" | "starter" | "pro" | "scale" | "enterprise";

type PricingFeature = {
  name: string;
  description?: string;
  included: boolean;
};

type UpgradeMode = "new_user" | "upgrade" | "checkout_recovery";
type PlanSlug = "free" | "starter" | "pro" | "scale" | "enterprise";

type BillingInterval = "monthly" | "yearly";

type PlanSelectionProps = {
  mode: UpgradeMode;
  isLoading?: boolean;
  onStartCheckout?: () => void;
  currentPlanSlug?: PlanSlug;
  selectedPlan: PlanType;
  billingInterval: BillingInterval;
  onPlanChange: (plan: PlanType) => void;
  onBillingIntervalChange: (interval: BillingInterval) => void;
  plans: PricingPlan[];
  comparisonFeatures: ComparisonFeature[];
};

const BILLING_INTERVALS = [
  { label: "Monthly", value: "monthly" as const },
  { label: "Yearly", value: "yearly" as const, helper: "Save ~17%" },
];

const MESSAGING: Record<
  UpgradeMode,
  { title: string; description: string; buttonText: string }
> = {
  new_user: {
    title: "Choose your plan",
    description:
      "All features included on every plan. Start free, upgrade when you need more.",
    buttonText: "Get started",
  },
  checkout_recovery: {
    title: "Complete your setup",
    description: "Pick up where you left off.",
    buttonText: "Subscribe now",
  },
  upgrade: {
    title: "Upgrade your plan",
    description: "Get more runs, more projects, and more team members.",
    buttonText: "Upgrade now",
  },
};

const PricingCardBadges = ({
  billingInterval,
  plan,
  isCurrentPlan,
}: {
  billingInterval: "monthly" | "yearly";
  plan: PricingPlan;
  isCurrentPlan?: boolean;
}) => {
  const renderLeftBadge = () => {
    if (isCurrentPlan) {
      return (
        <Badge
          className="absolute -top-2 left-2 flex items-center gap-1 shadow-sm"
          variant="success-light"
        >
          <HugeiconsIcon className="size-3" icon={CheckIcon} />
          <span className="font-normal text-xs">Current plan</span>
        </Badge>
      );
    }
    if (plan.badge) {
      return (
        <Badge
          className="absolute -top-2 left-2 flex items-center gap-1 shadow-sm"
          variant={plan.badgeVariant ?? "info-light"}
        >
          <span className="font-normal text-xs">{plan.badge}</span>
        </Badge>
      );
    }
    if (plan.highlight) {
      return (
        <Badge
          className="absolute -top-2 left-2 flex items-center gap-1 shadow-sm"
          variant="info-light"
        >
          <HugeiconsIcon className="size-3" icon={StarIcon} />
          <span className="font-normal text-xs">Most Popular</span>
        </Badge>
      );
    }
    return null;
  };

  return (
    <>
      {billingInterval === "yearly" &&
      plan.prices.monthly > 0 &&
      plan.prices.yearly > 0 ? (
        <Badge
          className="absolute -top-2 right-2 flex items-center gap-1 shadow-sm"
          variant="success-light"
        >
          <HugeiconsIcon className="size-3" icon={CheckIcon} />
          <span className="font-normal text-xs">
            Save{" "}
            {Math.round(
              ((plan.prices.monthly * MONTHS_IN_A_YEAR - plan.prices.yearly) /
                (plan.prices.monthly * MONTHS_IN_A_YEAR)) *
                PERCENTAGE_MULTIPLIER
            )}
            %
          </span>
        </Badge>
      ) : null}
      {renderLeftBadge()}
    </>
  );
};

const PricingCardFeatures = ({ plan }: { plan: PricingPlan }) => (
  <div className="mt-4 grow space-y-2">
    {plan.features.slice(0, 8).map((feature: PricingFeature) => (
      <div className="flex items-start gap-2" key={feature.name}>
        <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded text-foreground">
          <HugeiconsIcon className="size-3" icon={CheckIcon} />
        </div>
        <span className="text-muted-foreground/80 text-xs">
          {feature.description ? feature.description : feature.name}
        </span>
      </div>
    ))}
    {plan.features.length > 8 && (
      <div className="pt-1 text-center text-muted-foreground/60 text-xs">
        +{plan.features.length - 8} more features
      </div>
    )}
  </div>
);

const PricingCard = ({
  plan,
  billingInterval,
  isSelected,
  onSelect,
  onStartCheckout,
  isLoading,
  buttonText,
  currentPlanSlug,
}: {
  plan: PricingPlan;
  billingInterval: "monthly" | "yearly";
  isSelected: boolean;
  onSelect: (planSlug: PlanType) => void;
  onStartCheckout?: () => void;
  isLoading?: boolean;
  buttonText: string;
  currentPlanSlug?: PlanSlug;
}) => {
  const isCurrentPlan = currentPlanSlug === plan.slug;
  const isFreePlan = plan.slug === "free";
  const isEnterprise = plan.slug === "enterprise";

  const getCardButtonText = () => {
    if (isFreePlan) {
      return "Get Started Free";
    }
    if (isEnterprise) {
      return "Contact Sales";
    }
    if (isCurrentPlan) {
      return "Current Plan";
    }
    return isSelected ? buttonText : "Choose this plan";
  };

  const getButtonVariant = () => {
    if (isCurrentPlan) {
      return "outline" as const;
    }
    if (isSelected) {
      return "default" as const;
    }
    return "outline" as const;
  };

  const currentPrice =
    billingInterval === "monthly"
      ? plan.prices.monthly
      : plan.prices.monthlyInYearly ||
        (plan.prices.yearly > 0
          ? Math.floor(plan.prices.yearly / MONTHS_IN_A_YEAR)
          : 0);

  const handleCardClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      if (!(isLoading || isCurrentPlan)) {
        onSelect(plan.slug);
      }
    },
    [isLoading, isCurrentPlan, onSelect, plan.slug]
  );

  return (
    <button
      className={cn(
        "group relative w-full text-left",
        "rounded",
        "bg-card",
        "border-2",
        isSelected
          ? "border-foreground shadow-lg ring-2 ring-foreground/20"
          : "border-border hover:border-foreground/30",
        !!plan.highlight && !isSelected && "border-foreground/20"
      )}
      disabled={isLoading || isCurrentPlan}
      onClick={handleCardClick}
      type="button"
    >
      <PricingCardBadges
        billingInterval={billingInterval}
        isCurrentPlan={isCurrentPlan}
        plan={plan}
      />

      <div className="flex h-full flex-col p-4">
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h4 className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
              {plan.name}
            </h4>
            {isFreePlan || isEnterprise ? null : (
              <div
                className={cn(
                  "flex size-4 items-center justify-center rounded-full border-2 transition-colors",
                  isSelected
                    ? "border-foreground bg-foreground text-background"
                    : "border-muted-foreground/30"
                )}
              >
                {isSelected ? (
                  <HugeiconsIcon className="size-2.5" icon={CheckIcon} />
                ) : null}
              </div>
            )}
          </div>
          <div className="flex items-baseline gap-1">
            {plan.isCustomPricing ? (
              <span className="font-normal text-2xl text-foreground tracking-tighter">
                Custom
              </span>
            ) : (
              <>
                <span className="font-normal text-2xl text-foreground tracking-tighter">
                  <span className="tabular-nums">
                    {formatCurrency(currentPrice / CENTS_TO_DOLLARS)}
                  </span>
                </span>
                <span className="text-muted-foreground text-xs">/month</span>
              </>
            )}
          </div>
          {billingInterval === "yearly" &&
            plan.prices.yearly > 0 &&
            !plan.isCustomPricing && (
              <div className="-mt-1 text-muted-foreground/80 text-xs">
                <span className="tabular-nums">
                  {formatCurrency(plan.prices.yearly / CENTS_TO_DOLLARS)} billed
                  annually
                </span>
              </div>
            )}
          <p className="border-border border-b pb-3 text-muted-foreground text-xs">
            {plan.description}
          </p>
        </div>

        <PricingCardFeatures plan={plan} />

        <Button
          className="mt-4 w-full"
          disabled={isLoading || isCurrentPlan}
          onClick={(e) => {
            e.stopPropagation();
            if (isEnterprise) {
              window.location.assign("/app/enterprise-contact");
              return;
            }
            if (isFreePlan) {
              window.location.assign("/app");
              return;
            }
            if (isSelected) {
              onStartCheckout?.();
            } else {
              onSelect(plan.slug);
              onStartCheckout?.();
            }
          }}
          type="button"
          variant={getButtonVariant()}
        >
          {getCardButtonText()}
        </Button>
        {isEnterprise && currentPlanSlug === "scale" ? (
          <p className="mt-2 text-center text-[11px] text-muted-foreground/70">
            Your Scale subscription will be credited toward your Enterprise
            contract.
          </p>
        ) : null}
      </div>
    </button>
  );
};

export const PlanSelection = ({
  mode,
  isLoading,
  onStartCheckout,
  currentPlanSlug,
  selectedPlan,
  billingInterval,
  onPlanChange,
  onBillingIntervalChange,
  plans: pricingPlans,
  comparisonFeatures,
}: PlanSelectionProps) => {
  const { trackSubscription } = useAnalytics();

  const messaging = MESSAGING[mode];

  const handleBillingIntervalChange = useCallback(
    (interval: BillingInterval) => {
      onBillingIntervalChange(interval);
      trackSubscription("BILLING_INTERVAL_CHANGED", { interval });
    },
    [onBillingIntervalChange, trackSubscription]
  );

  const handlePlanSelect = useCallback(
    (planSlug: PlanType) => {
      if (!isLoading) {
        onPlanChange(planSlug);
      }
    },
    [isLoading, onPlanChange]
  );

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h1 className="text-balance font-normal text-secondary-foreground text-xl tracking-tight">
          {messaging.title}
        </h1>
        <p className="whitespace-normal text-muted-foreground text-sm">
          {messaging.description}
        </p>
      </div>

      {/* Billing Toggle */}
      <div className="flex justify-center">
        <Tabs
          onValueChange={(v) =>
            handleBillingIntervalChange(v as BillingInterval)
          }
          value={billingInterval}
        >
          <TabsList>
            {BILLING_INTERVALS.map((option) => (
              <TabsTrigger key={option.value} value={option.value}>
                {option.label}
                {option.helper ? (
                  <Badge variant="success-light">{option.helper}</Badge>
                ) : null}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {/* Plan Cards */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {pricingPlans.map((plan) => (
          <PricingCard
            billingInterval={billingInterval}
            buttonText={messaging.buttonText}
            currentPlanSlug={currentPlanSlug}
            isLoading={isLoading}
            isSelected={selectedPlan === plan.slug}
            key={plan.slug}
            onSelect={handlePlanSelect}
            onStartCheckout={onStartCheckout}
            plan={plan}
          />
        ))}
      </div>

      {/* Pricing Calculator */}
      <PricingCalculator />

      {/* No surprise bills callout */}
      <div className="mx-auto max-w-xl rounded border border-border bg-muted/30 p-4 text-center">
        <p className="font-medium text-foreground text-sm">No surprise bills</p>
        <p className="mt-1 text-muted-foreground text-xs">
          Set a spending limit on any paid plan. When you reach it, runs stop —
          you are never charged more than you expect. Free plan users are always
          hard-capped.
        </p>
      </div>

      {/* Compare link */}
      <div className="text-center">
        <a
          className="text-muted-foreground text-sm underline underline-offset-4 transition-colors hover:text-foreground"
          href="/app/pricing/compare"
        >
          Compare with competitors
        </a>
      </div>

      {/* Feature comparison matrix */}
      <FeatureComparisonMatrix features={comparisonFeatures} />

      {/* FAQ */}
      <PricingFAQ />
    </div>
  );
};

const FeatureCellValue = ({ value }: { value: string }) => {
  if (value === "Yes") {
    return (
      <HugeiconsIcon className="mx-auto size-4 text-success" icon={CheckIcon} />
    );
  }
  if (value === "-") {
    return <span className="text-muted-foreground/50">-</span>;
  }
  return <>{value}</>;
};

const FeatureComparisonMatrix = ({
  features,
}: {
  features: ComparisonFeature[];
}) => {
  const tiers = ["free", "starter", "pro", "enterprise"] as const;
  const tierLabels = {
    free: "Free",
    starter: "Starter",
    pro: "Pro",
    enterprise: "Enterprise",
  };

  return (
    <div className="mt-12">
      <h3 className="mb-6 text-balance text-center font-medium text-sm">
        Full feature comparison
      </h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b">
              <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                Feature
              </th>
              {tiers.map((tier) => (
                <th className="px-4 py-3 text-center font-medium" key={tier}>
                  {tierLabels[tier]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {features.map((feature) => (
              <tr className="border-border/50 border-b" key={feature.name}>
                <td className="py-3 pr-4 text-muted-foreground">
                  {feature.name}
                </td>
                {tiers.map((tier) => (
                  <td className="px-4 py-3 text-center" key={tier}>
                    <FeatureCellValue value={feature[tier]} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

const FAQ_ITEMS = [
  {
    question: "How does billing work?",
    answer:
      "You are billed monthly or annually based on your chosen plan. Each plan includes a compute credit allowance. Usage beyond the included credit is billed as overage at the plan's per-1K-runs rate.",
  },
  {
    question: "What happens if I exceed my plan limits?",
    answer:
      "If you set a spending limit, runs will stop when the limit is reached (or you'll be notified, depending on your setting). Daily run limits reset at midnight UTC. You can upgrade at any time to increase your limits.",
  },
  {
    question: "Can I change plans at any time?",
    answer:
      "Yes. Upgrades take effect immediately and you get access to higher limits right away. Downgrades take effect at the end of your current billing period so you don't lose access mid-cycle.",
  },
  {
    question: "What happens when I cancel?",
    answer:
      "Your plan remains active until the end of the current billing period. After that, your account reverts to the Free plan. Your data is retained according to the Free plan's retention policy.",
  },
  {
    question: "Is there a spending cap?",
    answer:
      "Yes. Every paid plan lets you set a spending limit to cap overage charges. Free plan users are always hard-capped at the included allowances with no overage possible.",
  },
  {
    question: "Do you offer enterprise pricing?",
    answer:
      "Yes. Enterprise plans include custom pricing, unlimited resources, SSO, SLA guarantees, and dedicated support. Contact us to discuss your needs.",
  },
];

const PricingFAQ = () => (
  <div className="mx-auto mt-12 max-w-2xl">
    <h3 className="mb-6 text-balance text-center font-medium text-sm">
      Frequently asked questions
    </h3>
    <Accordion className="w-full">
      {FAQ_ITEMS.map((item) => (
        <AccordionItem key={item.question}>
          <AccordionTrigger className="text-left text-sm">
            {item.question}
          </AccordionTrigger>
          <AccordionContent className="text-muted-foreground text-sm">
            {item.answer}
          </AccordionContent>
        </AccordionItem>
      ))}
    </Accordion>
  </div>
);

export type { BillingInterval, PlanSlug, PlanType, UpgradeMode };
