import { HugeiconsIcon } from "@hugeicons/react";
import { CheckIcon as RadixCheckIcon } from "@radix-ui/react-icons";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { CardCheckboxItem } from "@strait/ui/components/card-checkbox";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import { cn } from "@strait/ui/utils/index";
import { formatCurrency } from "@strait/utils/money";
import { useCallback, useState } from "react";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { CheckIcon, StarIcon } from "@/lib/icons";
import { PERCENTAGE_MULTIPLIER } from "@/utils/constants";

const MONTHS_IN_A_YEAR = 12;
const CENTS_TO_DOLLARS = 100;

type PlanType = "starter" | "growth" | "professional" | "enterprise";

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
};

type UpgradeMode = "new_user" | "trial_ended" | "checkout_recovery";
type PlanSlug = "starter" | "growth" | "professional" | "enterprise";

type BillingInterval = "monthly" | "yearly";

const PRICING_PLANS: PricingPlan[] = [
  {
    name: "Starter",
    slug: "starter",
    description: "Core tools for small teams starting their operations.",
    prices: {
      monthly: 2900,
      yearly: 27_840,
      monthlyInYearly: 2320,
    },
    features: [
      { name: "Product catalog", included: true },
      { name: "Basic inventory tracking", included: true },
      { name: "Sales and invoices", included: true },
      { name: "1 organization", included: true },
      { name: "Email support", included: true },
    ],
  },
  {
    name: "Growth",
    slug: "growth",
    description: "Automation and insights for scaling operations.",
    prices: {
      monthly: 5900,
      yearly: 56_640,
      monthlyInYearly: 4720,
    },
    includesFromPrevious: "Everything in Starter",
    differentialFeatures: [
      { name: "Advanced reports", included: true },
      { name: "Purchase workflows", included: true },
      { name: "Multi-location inventory", included: true },
      { name: "Workflow automations", included: true },
      { name: "Priority support", included: true },
    ],
    features: [],
    highlight: true,
  },
  {
    name: "Professional",
    slug: "professional",
    description: "Control, governance, and deeper visibility for bigger teams.",
    prices: {
      monthly: 9900,
      yearly: 95_040,
      monthlyInYearly: 7920,
    },
    includesFromPrevious: "Everything in Growth",
    differentialFeatures: [
      { name: "Role-based permissions", included: true },
      { name: "Approval flows", included: true },
      { name: "Audit trail", included: true },
      { name: "Custom exports", included: true },
      { name: "API access", included: true },
    ],
    features: [],
  },
  {
    name: "Enterprise",
    slug: "enterprise",
    description: "Enterprise security and tailored support for complex orgs.",
    prices: {
      monthly: 14_900,
      yearly: 143_040,
      monthlyInYearly: 11_920,
    },
    includesFromPrevious: "Everything in Professional",
    differentialFeatures: [
      { name: "SSO and SCIM", included: true },
      { name: "Dedicated onboarding", included: true },
      { name: "SLA-backed support", included: true },
      { name: "Custom integrations", included: true },
      { name: "Dedicated success manager", included: true },
    ],
    features: [],
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
  { label: "Yearly", value: "yearly" as const, helper: "Save 20%" },
];

const MESSAGING: Record<
  UpgradeMode,
  { title: string; description: string; buttonText: string }
> = {
  new_user: {
    title: "Choose your plan",
    description:
      "Start your 14-day free trial with full access to all features.",
    buttonText: "Start free trial",
  },
  checkout_recovery: {
    title: "Complete your setup",
    description:
      "Start your 14-day free trial with full access to all features.",
    buttonText: "Start free trial",
  },
  trial_ended: {
    title: "Your trial has ended",
    description: "Choose a plan to continue using Strait.",
    buttonText: "Subscribe now",
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
  // Determine which left badge to show (Current plan takes priority over Most Popular)
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
      {billingInterval === "yearly" ? (
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
      .slice(0, 5)
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
    {(plan.differentialFeatures || plan.features).length > 5 && (
      <div className="pt-1 text-center text-muted-foreground/60 text-xs">
        +{(plan.differentialFeatures || plan.features).length - 5} more features
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

  // Determine button text based on current plan status
  const getCardButtonText = () => {
    if (isCurrentPlan) {
      // User's current/trial plan - encourage them to subscribe
      return isSelected ? buttonText : "Subscribe to this plan";
    }
    if (currentPlanSlug) {
      // User has a plan but this is a different one
      return "Upgrade plan";
    }
    return isSelected ? buttonText : "Choose this plan";
  };

  // Determine button variant - selected plan gets primary, others get outline
  const getButtonVariant = () => {
    if (isSelected) {
      return "default";
    }
    return "outline";
  };
  const currentPrice =
    billingInterval === "monthly"
      ? plan.prices.monthly
      : plan.prices.monthlyInYearly ||
        Math.floor(plan.prices.yearly / MONTHS_IN_A_YEAR);

  const handleCardClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      if (!isLoading) {
        onSelect(plan.slug);
      }
    },
    [isLoading, onSelect, plan.slug]
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
        if ((e.key === "Enter" || e.key === " ") && !isLoading) {
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
            <div
              className={cn(
                "flex size-4 items-center justify-center rounded-full border-2 transition-colors",
                isSelected
                  ? "border-foreground bg-foreground text-background"
                  : "border-muted-foreground/30"
              )}
            >
              {isSelected ? <RadixCheckIcon className="h-2.5 w-2.5" /> : null}
            </div>
          </div>
          <div className="flex items-baseline gap-1">
            <span className="font-normal text-2xl text-foreground tracking-tighter">
              <span className="tabular-nums">
                {formatCurrency(currentPrice / CENTS_TO_DOLLARS)}
              </span>
            </span>
            <span className="text-muted-foreground text-xs">/month</span>
          </div>
          {billingInterval === "yearly" && (
            <div className="-mt-1 text-muted-foreground/80 text-xs">
              <span className="tabular-nums">
                {formatCurrency(plan.prices.yearly / CENTS_TO_DOLLARS)} billed
                annually
              </span>
            </div>
          )}
          <p className="border-border border-b pb-3 text-muted-foreground text-xs">
            {plan.description ||
              `Perfect for ${plan.name.toLowerCase()} businesses`}
          </p>
        </div>

        <PricingCardFeatures plan={plan} />

        <Button
          className="mt-4 w-full"
          disabled={isLoading}
          onClick={(e) => {
            e.stopPropagation();
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

  const [trialReminderOptIn, setTrialReminderOptIn] = useState(true);

  const messaging = MESSAGING[mode];

  const handleBillingIntervalChange = useCallback(
    (interval: BillingInterval) => {
      onBillingIntervalChange(interval);
      trackSubscription("BILLING_INTERVAL_CHANGED", { interval });
    },
    [onBillingIntervalChange, trackSubscription]
  );

  // Check if user already has a subscription
  const hasExistingSubscription = !!currentPlanSlug;

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

      {/* Trial Reminder Opt-in - only show for trial modes and when user doesn't have subscription */}
      {mode !== "trial_ended" && !hasExistingSubscription && (
        <div className="mx-auto max-w-xl">
          <CardCheckboxItem
            checked={trialReminderOptIn}
            description="We'll send you a friendly reminder a few days before your trial ends, so you have time to decide."
            id="trial-reminder"
            label="Email me a reminder before my trial ends"
            onCheckedChange={(checked) =>
              setTrialReminderOptIn(checked === true)
            }
          />
        </div>
      )}
    </div>
  );
};

export type { BillingInterval, PlanType, UpgradeMode };
