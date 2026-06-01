import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { NoticeBanner } from "@strait/ui/components/notice-banner";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { AlarmClockIcon, SparklesIcon } from "@/lib/icons";

const TRIAL_CRITICAL_DAYS = 2; // 1-2 days left = critical (destructive/red)
const TRIAL_WARNING_DAYS = 5; // 3-5 days left = warning (orange)

const TrialUpgradeCard = () => {
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { subscription, shouldShowUpgrade, isTrialing, trialDaysLeft } = data;

  const getTrialContent = useMemo(() => {
    // Default for no trial days info
    if (trialDaysLeft === null || trialDaysLeft <= 0) {
      return {
        title: "Premium Trial Active",
        message:
          "Enjoying unlimited Premium features during your trial period!",
        color: "green" as const,
      };
    }

    const daysText = trialDaysLeft === 1 ? "1 day" : `${trialDaysLeft} days`;
    const trialMessage = `${daysText} remaining. Subscribe now to keep your access!`;

    // Critical: 1-2 days left (red/destructive)
    if (trialDaysLeft <= TRIAL_CRITICAL_DAYS) {
      return {
        title:
          trialDaysLeft === 1
            ? "Trial ends tomorrow!"
            : "Trial ending very soon!",
        message: trialMessage,
        color: "red" as const,
      };
    }

    // Warning: 3-5 days left (orange)
    if (trialDaysLeft <= TRIAL_WARNING_DAYS) {
      return {
        title: "Trial ending soon!",
        message: trialMessage,
        color: "orange" as const,
      };
    }

    // Normal: 6+ days left (green)
    return {
      title: "Premium Trial Active",
      message: trialMessage,
      color: "green" as const,
    };
  }, [trialDaysLeft]);

  const getSubscriptionContent = useMemo(() => {
    if (!subscription) {
      return {
        title: "You don't have an active subscription",
        message: "Upgrade to access all Strait features.",
        color: "blue" as const,
      };
    }

    switch (subscription.status) {
      case "incomplete":
        return {
          title: "Your subscription needs attention",
          message:
            "Your payment is pending. Complete the payment to continue using Strait.",
          color: "orange" as const,
        };
      case "past_due":
        return {
          title: "Your subscription needs attention",
          message:
            "Your payment is overdue. Update your payment method to continue using Strait.",
          color: "red" as const,
        };
      case "unpaid":
        return {
          title: "Your subscription needs attention",
          message:
            "Your payment was not processed. Update your payment method to continue using Strait.",
          color: "red" as const,
        };
      case "canceled":
        return {
          title: "Your subscription has been canceled",
          message: "Upgrade to continue using Strait.",
          color: "red" as const,
        };
      default:
        return { title: "", message: "", color: "blue" as const };
    }
  }, [subscription]);

  // Determine card type and content - memoized to prevent complex conditional logic on every render
  const cardContent = useMemo(() => {
    if (isTrialing || subscription?.status === "trialing") {
      return getTrialContent;
    }
    return getSubscriptionContent;
  }, [subscription, isTrialing, getTrialContent, getSubscriptionContent]);

  const bannerConfig = useMemo(
    () => ({
      blue: {
        variant: "info" as const,
        buttonVariant: "default" as const,
        icon: SparklesIcon,
      },
      green: {
        variant: "success" as const,
        buttonVariant: "default" as const,
        icon: SparklesIcon,
      },
      orange: {
        variant: "warning" as const,
        buttonVariant: "warning-solid" as const,
        icon: AlarmClockIcon,
      },
      red: {
        variant: "destructive" as const,
        buttonVariant: "destructive-solid" as const,
        icon: AlarmClockIcon,
      },
    }),
    []
  );

  const { title, message, color } = cardContent;
  const config = bannerConfig[color];

  // Don't render anything if we shouldn't show the card (use hook value)
  if (!shouldShowUpgrade) {
    return null;
  }

  return (
    <NoticeBanner
      action={
        <Button
          render={<Link preload="intent" to="/app/upgrade" />}
          variant={config.buttonVariant}
        >
          Upgrade
        </Button>
      }
      icon={<HugeiconsIcon className="size-4" icon={config.icon} />}
      title={title}
      variant={config.variant}
    >
      {message}
    </NoticeBanner>
  );
};

export default TrialUpgradeCard;
