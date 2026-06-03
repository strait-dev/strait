import { Button } from "@strait/ui/components/button";
import {
  NoticeBanner,
  NoticeBannerAction,
} from "@strait/ui/components/notice-banner";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";

const MICRO_USD = 1_000_000;
const FREE_PLAN = "free";

const OverageWarningBanner = () => {
  const { data } = useQuery(orgUsageQueryOptions());
  const navigate = useNavigate();

  const orgId = data?.org_id ?? "";
  const periodStart = data?.period?.start ?? "";

  // Only build a meaningful storage key when we have real data.
  const storageKey =
    orgId && periodStart
      ? `strait:overage_dismissed:${orgId}:${periodStart}`
      : "";

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined" || !storageKey) {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  if (!data || dismissed) {
    return null;
  }

  const plan = data.plan ?? FREE_PLAN;
  const overageMicro = data.overage_microusd ?? 0;

  // Free orgs do not show paid-plan overage banners here.
  if (plan === FREE_PLAN) {
    return null;
  }

  const overageDollars = (overageMicro / MICRO_USD).toFixed(2);

  const isInOverage = overageMicro > 0;

  if (!isInOverage) {
    return null;
  }

  const handleDismiss = () => {
    setDismissed(true);
    if (storageKey) {
      localStorage.setItem(storageKey, "true");
    }
  };

  if (isInOverage) {
    return (
      <NoticeBanner
        action={
          <NoticeBannerAction>
            <Button
              onClick={() => navigate({ to: "/app/billing" })}
              variant="destructive"
            >
              Set limit
            </Button>
          </NoticeBannerAction>
        }
        dismissible
        onDismiss={handleDismiss}
        title="Included run allowance exceeded"
        variant="destructive"
      >
        You're <strong>${overageDollars}</strong> beyond your included run
        allowance. Set a spending cap to control costs.
      </NoticeBanner>
    );
  }

  return null;
};

export default OverageWarningBanner;
