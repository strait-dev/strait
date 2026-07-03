import { formatPrice, PLANS } from "@strait/billing";
import { Button } from "@strait/ui/components/button";
import {
  NoticeBanner,
  NoticeBannerAction,
} from "@strait/ui/components/notice-banner";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useState } from "react";
import {
  orgUsageQueryOptions,
  useApproachingLimits,
} from "@/hooks/billing/use-org-usage";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";

const MICRO_USD = 1_000_000;
const CENTS_TO_MICRO_USD = 10_000;
const PRO_MONTHLY_PRICE_CENTS = PLANS.pro.prices.monthly;
const SCALE_MONTHLY_PRICE_CENTS = PLANS.scale.prices.monthly;
const PRO_MONTHLY_PRICE_MICRO_USD =
  PRO_MONTHLY_PRICE_CENTS * CENTS_TO_MICRO_USD;
const SCALE_MONTHLY_PRICE_MICRO_USD =
  SCALE_MONTHLY_PRICE_CENTS * CENTS_TO_MICRO_USD;
const SCALE_APPROACH_THRESHOLD_MICRO_USD = Math.floor(
  SCALE_MONTHLY_PRICE_MICRO_USD * 0.9
);
const SCALE_MONTHLY_PRICE = formatPrice(SCALE_MONTHLY_PRICE_CENTS);

const UpgradeNudgeBanner = () => {
  const approaching = useApproachingLimits();
  const { data: usageData } = useQuery(orgUsageQueryOptions());
  const { data: forecast } = useQuery(usageForecastQueryOptions());
  const navigate = useNavigate();

  const orgId = usageData?.org_id ?? "";
  const periodStart = usageData?.period?.start ?? "";
  const storageKey =
    orgId && periodStart
      ? `strait:banner_dismissed:${orgId}:${periodStart}`
      : "";

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined" || !storageKey) {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  const handleUpgrade = () => {
    navigate({ to: "/app/upgrade" });
  };

  const handleDismiss = () => {
    setDismissed(true);
    if (storageKey) {
      localStorage.setItem(storageKey, "true");
    }
  };

  const renderUpgradeBanner = ({
    children,
    cta = "Upgrade",
    title,
    variant = "warning",
  }: {
    children: ReactNode;
    cta?: string;
    title: string;
    variant?: "info" | "warning";
  }) => (
    <NoticeBanner
      action={
        <NoticeBannerAction>
          <Button onClick={handleUpgrade}>{cta}</Button>
        </NoticeBannerAction>
      }
      dismissible
      onDismiss={handleDismiss}
      title={title}
      variant={variant}
    >
      {children}
    </NoticeBanner>
  );

  if (dismissed) {
    return null;
  }

  const currentPlan = usageData?.plan;

  // Priority 1: Scale breakeven -- only show to Pro users.
  if (forecast?.scale_breakeven && currentPlan === "pro") {
    return renderUpgradeBanner({
      cta: "Upgrade to Scale",
      title: "Scale plan recommended",
      variant: "info",
      children: (
        <>
          Your projected spend exceeds {SCALE_MONTHLY_PRICE}/mo. Upgrade to{" "}
          <strong>Scale</strong> for more included orchestration runs, 3x
          concurrent runs, audit logs, and canary deploys.
        </>
      ),
    });
  }

  // Priority 2: Addon spend tipping point for Pro users.
  if (
    forecast?.addon_spend_microusd &&
    forecast.addon_spend_microusd > 0 &&
    currentPlan === "pro"
  ) {
    const addonSpend = (forecast.addon_spend_microusd / MICRO_USD).toFixed(2);
    const totalSpend = (
      (PRO_MONTHLY_PRICE_MICRO_USD + forecast.addon_spend_microusd) /
      MICRO_USD
    ).toFixed(2);
    if (
      PRO_MONTHLY_PRICE_MICRO_USD + forecast.addon_spend_microusd >=
      SCALE_APPROACH_THRESHOLD_MICRO_USD
    ) {
      return renderUpgradeBanner({
        cta: "Upgrade to Scale",
        title: "Scale plan recommended",
        variant: "info",
        children: (
          <>
            You're spending <strong>${totalSpend}/mo</strong> on Pro + add-ons
            (${addonSpend} in add-ons). Scale ({SCALE_MONTHLY_PRICE}/mo)
            includes higher launch limits.
          </>
        ),
      });
    }
  }

  // Priority 3: Projected overage warning for Pro users.
  if (
    forecast?.projected_overage_microusd &&
    forecast.projected_overage_microusd > 0 &&
    currentPlan === "pro"
  ) {
    const projectedOverage = (
      forecast.projected_overage_microusd / MICRO_USD
    ).toFixed(2);
    return renderUpgradeBanner({
      title: "Projected overage",
      children: (
        <>
          Your projected overage this month is{" "}
          <strong>${projectedOverage}</strong>. Consider upgrading for more
          included orchestration runs.
        </>
      ),
    });
  }

  // Priority 3: Existing approaching limit alerts.
  if (!approaching.length) {
    return null;
  }

  const firstAlert = approaching[0];

  return renderUpgradeBanner({
    title: "Usage limit approaching",
    children: firstAlert.message,
  });
};

export default UpgradeNudgeBanner;
