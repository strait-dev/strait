import { HugeiconsIcon } from "@hugeicons/react";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Card, CardContent } from "@strait/ui/components/card";
import { NoticeBanner } from "@strait/ui/components/notice-banner";
import { Separator } from "@strait/ui/components/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import PricingCalculator from "@/components/upgrade/pricing-calculator";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { formatCurrency } from "@/lib/format";
import { CheckIcon, StarIcon } from "@/lib/icons";
import { PERCENTAGE_MULTIPLIER } from "@/utils/constants";

const MONTHS_IN_A_YEAR = 12;
const CENTS_TO_DOLLARS = 100;

import type { ComparisonFeature, PricingPlan } from "@/hooks/billing/use-plans";

type PlanType =
  | "free"
  | "starter"
  | "pro"
  | "scale"
  | "business"
  | "enterprise";

type PricingFeature = {
  name: string;
  description?: string;
  included: boolean;
};

type UpgradeMode = "new_user" | "upgrade" | "checkout_recovery";
type PlanSlug =
  | "free"
  | "starter"
  | "pro"
  | "scale"
  | "business"
  | "enterprise";

type BillingInterval = "monthly" | "yearly";

type PlanSelectionProps = {
  mode: UpgradeMode;
  isLoading?: boolean;
  onStartCheckout?: (planSlug?: PlanType) => void;
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
    description: "Start free, then upgrade when you need higher launch limits.",
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
}) => (
  <>
    {billingInterval === "yearly" &&
    plan.prices.monthly > 0 &&
    plan.prices.yearly > 0 ? (
      <Badge
        className="absolute -top-2 right-2"
        iconLeft={CheckIcon}
        variant="success-light"
      >
        Save{" "}
        {Math.round(
          ((plan.prices.monthly * MONTHS_IN_A_YEAR - plan.prices.yearly) /
            (plan.prices.monthly * MONTHS_IN_A_YEAR)) *
            PERCENTAGE_MULTIPLIER
        )}
        %
      </Badge>
    ) : null}
    <PricingCardLeftBadge isCurrentPlan={isCurrentPlan} plan={plan} />
  </>
);

const PricingCardLeftBadge = ({
  isCurrentPlan,
  plan,
}: {
  isCurrentPlan?: boolean;
  plan: PricingPlan;
}) => {
  if (isCurrentPlan) {
    return (
      <Badge
        className="absolute -top-2 left-2"
        iconLeft={CheckIcon}
        variant="success-light"
      >
        Current plan
      </Badge>
    );
  }
  if (plan.badge) {
    return (
      <Badge
        className="absolute -top-2 left-2"
        variant={plan.badgeVariant ?? "info-light"}
      >
        {plan.badge}
      </Badge>
    );
  }
  if (plan.highlight) {
    return (
      <Badge
        className="absolute -top-2 left-2"
        iconLeft={StarIcon}
        variant="info-light"
      >
        Most popular
      </Badge>
    );
  }
  return null;
};

const PricingCardFeatures = ({ plan }: { plan: PricingPlan }) => (
  <div className="mt-4 grow space-y-2">
    {plan.features.slice(0, 8).map((feature: PricingFeature) => (
      <div className="flex items-start gap-2" key={feature.name}>
        <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center text-foreground">
          <HugeiconsIcon className="size-3" icon={CheckIcon} />
        </div>
        <span className="text-muted-foreground text-xs">
          {feature.description ? feature.description : feature.name}
        </span>
      </div>
    ))}
    {plan.features.length > 8 && (
      <div className="pt-1 text-center text-muted-foreground text-xs">
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
  onStartCheckout?: (planSlug?: PlanType) => void;
  isLoading?: boolean;
  buttonText: string;
  currentPlanSlug?: PlanSlug;
}) => {
  const isCurrentPlan = currentPlanSlug === plan.slug;
  const isFreePlan = plan.slug === "free";
  const isEnterprise = plan.slug === "enterprise";

  const getCardButtonText = () => {
    if (isFreePlan) {
      return "Get started free";
    }
    if (isEnterprise) {
      return "Contact sales";
    }
    if (isCurrentPlan) {
      return "Current plan";
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

  const handleCardClick = (e: React.MouseEvent) => {
    e.preventDefault();
    if (!(isLoading || isCurrentPlan)) {
      onSelect(plan.slug);
    }
  };

  const handleCardKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "Enter" && e.key !== " ") {
      return;
    }
    e.preventDefault();
    if (!(isLoading || isCurrentPlan)) {
      onSelect(plan.slug);
    }
  };

  return (
    <Card
      aria-disabled={isLoading || isCurrentPlan}
      aria-pressed={isSelected}
      className="relative h-full cursor-pointer"
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      role="button"
      tabIndex={isLoading || isCurrentPlan ? -1 : 0}
      variant={isSelected ? "outline" : "default"}
    >
      <PricingCardBadges
        billingInterval={billingInterval}
        isCurrentPlan={isCurrentPlan}
        plan={plan}
      />

      <CardContent className="flex h-full flex-col">
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h4 className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
              {plan.name}
            </h4>
            {isSelected && !isFreePlan && !isEnterprise ? (
              <Badge size="xs" variant="success-light">
                Selected
              </Badge>
            ) : null}
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
              <div className="-mt-1 text-muted-foreground text-xs">
                <span className="tabular-nums">
                  {formatCurrency(plan.prices.yearly / CENTS_TO_DOLLARS)} billed
                  annually
                </span>
              </div>
            )}
          <p className="text-muted-foreground text-xs">{plan.description}</p>
          <Separator />
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
              onStartCheckout?.(plan.slug);
            } else {
              onSelect(plan.slug);
              onStartCheckout?.(plan.slug);
            }
          }}
          type="button"
          variant={getButtonVariant()}
        >
          {getCardButtonText()}
        </Button>
        {isEnterprise && currentPlanSlug === "scale" ? (
          <p className="mt-2 text-center text-muted-foreground text-xs">
            Your Scale subscription will be credited toward your Enterprise
            contract.
          </p>
        ) : null}
      </CardContent>
    </Card>
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

  const handleBillingIntervalChange = (interval: BillingInterval) => {
    onBillingIntervalChange(interval);
    trackSubscription("BILLING_INTERVAL_CHANGED", { interval });
  };

  const handlePlanSelect = (planSlug: PlanType) => {
    if (!isLoading) {
      onPlanChange(planSlug);
    }
  };

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
      <NoticeBanner
        className="mx-auto max-w-xl"
        title="No surprise bills"
        variant="info"
      >
        Set a spending limit on any paid plan. When you reach it, runs stop, you
        are never charged more than you expect. Free plan users are always
        hard-capped.
      </NoticeBanner>

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
      <Badge iconLeft={CheckIcon} size="xs" variant="success-light">
        Yes
      </Badge>
    );
  }
  if (value === "-") {
    return <span className="text-muted-foreground">-</span>;
  }
  return <>{value}</>;
};

const FeatureComparisonMatrix = ({
  features,
}: {
  features: ComparisonFeature[];
}) => {
  const tiers = [
    "free",
    "starter",
    "pro",
    "scale",
    "business",
    "enterprise",
  ] as const;
  const tierLabels = {
    free: "Free",
    starter: "Starter",
    pro: "Pro",
    scale: "Scale",
    business: "Business",
    enterprise: "Enterprise",
  };

  return (
    <div className="mt-12">
      <h3 className="mb-6 text-balance text-center font-medium text-sm">
        Full feature comparison
      </h3>
      <Table size="lg">
        <TableHeader>
          <TableRow>
            <TableHead scope="col">Feature</TableHead>
            {tiers.map((tier) => (
              <TableHead className="text-center" key={tier} scope="col">
                {tierLabels[tier]}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {features.map((feature) => (
            <TableRow key={feature.name}>
              <TableCell className="text-muted-foreground">
                {feature.name}
              </TableCell>
              {tiers.map((tier) => (
                <TableCell className="text-center" key={tier}>
                  <FeatureCellValue value={feature[tier]} />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
};

const FAQ_ITEMS = [
  {
    question: "How does billing work?",
    answer:
      "You are billed monthly or annually based on your chosen plan. Each plan includes a monthly orchestration-run allowance. Usage beyond that allowance is billed as overage at the plan's per-1K-runs rate.",
  },
  {
    question: "What happens if I exceed my plan limits?",
    answer:
      "If your spending cap uses the reject action, new dispatch stops and schedules pause when the cap is reached. Notify-only caps keep dispatching and alert you instead. Monthly run allowances reset at the start of each billing period.",
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
      "Yes. Every paid plan lets you set a spending cap for overage charges. Free plan users are hard-capped at the included allowance unless overage is enabled with a card on file.",
  },
  {
    question: "Do you offer enterprise pricing?",
    answer:
      "Yes. Enterprise plans include custom pricing, custom launch limits, non-contractual SLA targets, and dedicated support. SSO and advanced network controls are roadmap/contact-sales items for launch, not included launch entitlements.",
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
