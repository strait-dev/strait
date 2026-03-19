import { HugeiconsIcon } from "@hugeicons/react";
import { CheckIcon as RadixCheckIcon } from "@radix-ui/react-icons";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import { cn } from "@strait/ui/utils/index";
import { formatCurrency } from "@strait/utils/money";
import { useCallback } from "react";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { CheckIcon, StarIcon } from "@/lib/icons";
import { PERCENTAGE_MULTIPLIER } from "@/utils/constants";

const MONTHS_IN_A_YEAR = 12;
const CENTS_TO_DOLLARS = 100;

type PlanType = "free" | "starter" | "pro" | "enterprise";

type PricingFeature = {
  name: string;
  description?: string;
  included: boolean;
};

type PricingPlan = {
  name: string;
  slug: PlanType;
  description: string;
  prices: {
    monthly: number;
    yearly: number;
    monthlyInYearly?: number;
  };
  features: PricingFeature[];
  differentialFeatures?: PricingFeature[];
  includesFromPrevious?: string;
  highlight?: boolean;
  badge?: string;
  badgeVariant?: "success-light" | "info-light" | "default";
  isCustomPricing?: boolean;
};

type UpgradeMode = "new_user" | "upgrade" | "checkout_recovery";
type PlanSlug = "free" | "starter" | "pro" | "enterprise";

type BillingInterval = "monthly" | "yearly";

const PRICING_PLANS: PricingPlan[] = [
  {
    name: "Free",
    slug: "free",
    description: "For side projects and evaluation. All features included.",
    badge: "No credit card required",
    badgeVariant: "success-light",
    prices: {
      monthly: 0,
      yearly: 0,
    },
    features: [
      { name: "All core features", included: true },
      { name: "5,000 runs/day", included: true },
      { name: "100 managed runs/mo (micro, 10s)", included: true },
      { name: "1 organization", included: true },
      { name: "2 projects", included: true },
      { name: "3 members", included: true },
      { name: "1-day retention", included: true },
      { name: "Community support", included: true },
    ],
  },
  {
    name: "Starter",
    slug: "starter",
    description: "For growing teams with production workloads.",
    prices: {
      monthly: 1999,
      yearly: 19_999,
      monthlyInYearly: 1667,
    },
    features: [
      { name: "All core features", included: true },
      { name: "25,000 runs/day", included: true },
      { name: "$19.99/mo compute credit", included: true },
      { name: "25 concurrent runs", included: true },
      { name: "2 organizations", included: true },
      { name: "5 projects per org", included: true },
      { name: "10 members per org", included: true },
      { name: "7-day retention", included: true },
      { name: "6 regions", included: true },
      { name: "Basic RBAC", included: true },
      { name: "Email support (48h)", included: true },
    ],
    highlight: true,
  },
  {
    name: "Pro",
    slug: "pro",
    description: "For production workloads at scale.",
    prices: {
      monthly: 4999,
      yearly: 49_999,
      monthlyInYearly: 4167,
    },
    features: [
      { name: "All core features", included: true },
      { name: "100,000 runs/day", included: true },
      { name: "$49.99/mo compute credit", included: true },
      { name: "100 concurrent runs", included: true },
      { name: "5 organizations", included: true },
      { name: "15 projects per org", included: true },
      { name: "25 members per org", included: true },
      { name: "30-day retention", included: true },
      { name: "All regions", included: true },
      { name: "Full RBAC", included: true },
      { name: "Audit logs", included: true },
      { name: "AI Assistant BYOK", included: true },
      { name: "Priority support (24h)", included: true },
    ],
  },
  {
    name: "Enterprise",
    slug: "enterprise",
    description: "Custom everything for large organizations.",
    isCustomPricing: true,
    prices: {
      monthly: 0,
      yearly: 0,
    },
    features: [
      { name: "Unlimited everything", included: true },
      { name: "Custom compute credits", included: true },
      { name: "SSO/SAML", included: true },
      { name: "99.9% SLA", included: true },
      { name: "90-day retention", included: true },
      { name: "Dedicated support + Slack", included: true },
      { name: "Custom integrations", included: true },
      { name: "Static IPs", included: true },
    ],
  },
];

type PlanSelectionProps = {
  mode: UpgradeMode;
  isLoading?: boolean;
  onStartCheckout?: () => void;
  currentPlanSlug?: PlanSlug;
  selectedPlan: PlanType;
  billingInterval: BillingInterval;
  onPlanChange: (plan: PlanType) => void;
  onBillingIntervalChange: (interval: BillingInterval) => void;
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
    {plan.includesFromPrevious ? (
      <div className="flex items-start gap-2 border-border/50 border-b pb-2">
        <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-custom text-foreground">
          <RadixCheckIcon className="size-3" />
        </div>
        <span className="font-medium text-muted-foreground/80 text-xs">
          {plan.includesFromPrevious}
        </span>
      </div>
    ) : null}

    {(plan.differentialFeatures || plan.features)
      .slice(0, 8)
      .map((feature: PricingFeature) => (
        <div className="flex items-start gap-2" key={feature.name}>
          <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-custom text-foreground">
            <RadixCheckIcon className="size-3" />
          </div>
          <span className="text-muted-foreground/80 text-xs">
            {feature.description ? feature.description : feature.name}
          </span>
        </div>
      ))}
    {(plan.differentialFeatures || plan.features).length > 8 && (
      <div className="pt-1 text-center text-muted-foreground/60 text-xs">
        +{(plan.differentialFeatures || plan.features).length - 8} more features
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
    if (isFreePlan) { return "Get Started Free"; }
    if (isEnterprise) { return "Contact Sales"; }
    if (isCurrentPlan) { return "Current Plan"; }
    return isSelected ? buttonText : "Choose this plan";
  };

  const getButtonVariant = () => {
    if (isCurrentPlan) { return "outline" as const; }
    if (isSelected) { return "default" as const; }
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
    // biome-ignore lint/a11y/useSemanticElements: Using div for grid layout compatibility
    <div
      className={cn(
        "group relative w-full",
        "rounded-custom transition-all duration-300",
        "bg-card",
        "border-2",
        isSelected
          ? "border-foreground shadow-lg ring-2 ring-foreground/20"
          : "border-border hover:border-foreground/30",
        !!plan.highlight && !isSelected && "border-foreground/20"
      )}
      onClick={handleCardClick}
      onKeyDown={(e) => {
        if (
          (e.key === "Enter" || e.key === " ") &&
          !isLoading &&
          !isCurrentPlan
        ) {
          e.preventDefault();
          onSelect(plan.slug);
        }
      }}
      role="button"
      tabIndex={0}
    >
      <PricingCardBadges
        billingInterval={billingInterval}
        isCurrentPlan={isCurrentPlan}
        plan={plan}
      />

      <div className="flex h-full flex-col p-4">
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
              {plan.name}
            </h3>
            {(isFreePlan || isEnterprise ) ? null : (
              <div
                className={cn(
                  "flex size-4 items-center justify-center rounded-full border-2 transition-colors",
                  isSelected
                    ? "border-foreground bg-foreground text-background"
                    : "border-muted-foreground/30"
                )}
              >
                {isSelected ? (
                  <RadixCheckIcon className="h-2.5 w-2.5" />
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
              window.open("mailto:sales@strait.dev", "_blank");
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
          size="sm"
          type="button"
          variant={getButtonVariant()}
        >
          {getCardButtonText()}
        </Button>
      </div>
    </div>
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
        <h1 className="font-normal text-secondary-foreground text-xl tracking-tight">
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
        {PRICING_PLANS.map((plan) => (
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

      {/* No surprise bills callout */}
      <div className="mx-auto max-w-xl rounded-custom border border-border bg-muted/30 p-4 text-center">
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
          href="/app/pricing/compare"
          className="text-muted-foreground text-sm underline underline-offset-4 transition-colors hover:text-foreground"
        >
          Compare with competitors
        </a>
      </div>
    </div>
  );
};

export type { BillingInterval, PlanType, UpgradeMode, PlanSlug };
