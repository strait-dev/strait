import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { AlarmClockIcon, SparklesIcon } from "@/lib/icons";

const TRIAL_CRITICAL_DAYS = 2;
const TRIAL_WARNING_DAYS = 5;
const ATTENTION_STATUSES = new Set(["incomplete", "past_due", "unpaid"]);

const UpgradeBanner = () => {
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { subscription, shouldShowUpgrade, isTrialing, trialDaysLeft } = data;

  // All useMemo hooks must be called before any early returns

  const getTrialBannerInfo = useMemo(() => {
    // Default banner for no trial days info
    if (trialDaysLeft === null || trialDaysLeft <= 0) {
      return {
        title: "Premium Trial Active",
        message:
          "You have unlimited access to all Premium features - 5 stores, 10 team members, advanced automation & more!",
        color: "green" as const,
        icon: "clock",
      };
    }

    const daysText = trialDaysLeft === 1 ? "1 day" : `${trialDaysLeft} days`;
    const message = `${daysText} remaining. Subscribe now to keep your access!`;

    // Critical: 1-2 days left (red/destructive)
    if (trialDaysLeft <= TRIAL_CRITICAL_DAYS) {
      return {
        title:
          trialDaysLeft === 1
            ? "Trial ends tomorrow!"
            : "Trial ending very soon!",
        message,
        color: "red" as const,
        icon: "clock",
      };
    }

    // Warning: 3-5 days left (orange)
    if (trialDaysLeft <= TRIAL_WARNING_DAYS) {
      return {
        title: "Premium trial ending soon!",
        message,
        color: "orange" as const,
        icon: "clock",
      };
    }

    // Normal: 6+ days left (green)
    return {
      title: "Premium Trial Active",
      message,
      color: "green" as const,
      icon: "clock",
    };
  }, [trialDaysLeft]);

  const getSubscriptionBannerInfo = useMemo(() => {
    if (!subscription) {
      return {
        title: "Get started with Strait",
        message:
          "Choose a plan to unlock all powerful features and grow your business.",
        color: "blue",
        icon: "sparkles",
      };
    }

    if (subscription.status === "canceled") {
      return {
        title: "Your subscription has been canceled",
        message:
          "Reactivate your subscription to continue using Strait features.",
        color: "red",
        icon: "sparkles",
      };
    }

    if (ATTENTION_STATUSES.has(subscription.status)) {
      return {
        title: "Your subscription needs attention",
        message: "Update your payment method to continue using Strait.",
        color: "orange",
        icon: "sparkles",
      };
    }

    return {
      title: "Upgrade your plan",
      message: "Unlock advanced features to grow your business.",
      color: "blue",
      icon: "sparkles",
    };
  }, [subscription]);

  // Determine banner message based on status
  const bannerInfo = useMemo(() => {
    if (isTrialing || subscription?.status === "trialing") {
      return getTrialBannerInfo;
    }
    return getSubscriptionBannerInfo;
  }, [subscription, isTrialing, getTrialBannerInfo, getSubscriptionBannerInfo]);

  // Memoize color classes object to prevent recreation on every render
  const colorClasses = useMemo(
    () => ({
      blue: {
        bg: "bg-muted",
        border: "border-border",
        text: "text-foreground",
        titleText: "text-foreground",
        icon: "bg-muted text-foreground",
        buttonVariant: "default" as const,
        buttonClass: "",
      },
      green: {
        bg: "bg-muted",
        border: "border-border",
        text: "text-foreground",
        titleText: "text-foreground",
        icon: "bg-muted text-foreground",
        buttonVariant: "default" as const,
        buttonClass: "",
      },
      orange: {
        bg: "bg-orange-50 dark:bg-orange-950/30",
        border: "border-orange-200 dark:border-orange-800",
        text: "text-orange-700 dark:text-orange-300",
        titleText: "text-orange-800 dark:text-orange-200",
        icon: "bg-orange-100 text-orange-600 dark:bg-orange-900 dark:text-orange-400",
        buttonVariant: "secondary" as const,
        buttonClass:
          "bg-orange-500 text-white hover:bg-orange-600 dark:bg-orange-600 dark:hover:bg-orange-700",
      },
      red: {
        bg: "bg-destructive/15",
        border: "border-destructive/30",
        text: "text-destructive dark:text-red-400",
        titleText: "text-destructive dark:text-red-300 font-normal",
        icon: "bg-destructive/20 text-destructive",
        buttonVariant: "destructive" as const,
        buttonClass: "",
      },
    }),
    []
  );

  // Memoize color selection
  const colors = useMemo(
    () => colorClasses[bannerInfo.color as keyof typeof colorClasses],
    [colorClasses, bannerInfo.color]
  );

  // Memoize button text
  const buttonText = useMemo(
    () =>
      subscription?.status === "canceled" ||
      ATTENTION_STATUSES.has(subscription?.status || "")
        ? "Fix Subscription"
        : "Upgrade",
    [subscription?.status]
  );

  // Don't show banner if user has active subscription
  if (!shouldShowUpgrade) {
    return null;
  }

  return (
    <div
      className={`border-b py-3 ${colors.bg} ${colors.border} ${colors.text}`}
    >
      <div className="mx-auto flex w-full max-w-[1800px] gap-2 px-4 sm:px-8 md:items-center lg:px-20">
        <div className="flex grow gap-3 md:items-center">
          <div
            aria-hidden="true"
            className={`flex size-9 shrink-0 items-center justify-center rounded max-md:mt-0.5 ${colors.icon}`}
          >
            {bannerInfo.icon === "clock" ? (
              <HugeiconsIcon className="opacity-80" icon={AlarmClockIcon} />
            ) : (
              <HugeiconsIcon className="opacity-80" icon={SparklesIcon} />
            )}
          </div>
          <div className="flex grow flex-col justify-between gap-3 md:flex-row md:items-center">
            <div className="space-y-0.5">
              <p className={`font-medium text-sm ${colors.titleText}`}>
                {bannerInfo.title}
              </p>
              <p className={`text-sm ${colors.text}`}>{bannerInfo.message}</p>
            </div>
            <div className="flex gap-2 max-md:flex-wrap">
              <Button
                className={`text-sm shadow-sm ${colors.buttonClass}`}
                render={<Link preload="intent" to="/app/upgrade" />}
                size="sm"
                variant={colors.buttonVariant}
              >
                {buttonText}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default UpgradeBanner;
