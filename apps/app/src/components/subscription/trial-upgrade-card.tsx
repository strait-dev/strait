import { AlarmClockIcon, SparklesIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { cn } from "@strait/ui/utils/index.ts";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription.ts";

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

  // Color classes matching the banner styles
  const colorClasses = useMemo(
    () => ({
      blue: {
        container: "bg-sidebar-accent border-sidebar-border",
        title: "text-sidebar-foreground",
        message: "text-sidebar-foreground/70",
        buttonVariant: "default" as const,
        buttonClass: "",
        icon: SparklesIcon,
      },
      green: {
        container: "bg-sidebar-accent border-sidebar-border",
        title: "text-sidebar-foreground",
        message: "text-sidebar-foreground/70",
        buttonVariant: "default" as const,
        buttonClass: "",
        icon: SparklesIcon,
      },
      orange: {
        container:
          "bg-orange-50 border-orange-200 dark:bg-orange-950/30 dark:border-orange-800",
        title: "text-orange-800 dark:text-orange-200",
        message: "text-orange-700 dark:text-orange-300",
        buttonVariant: "secondary" as const,
        buttonClass:
          "bg-orange-500 text-white hover:bg-orange-600 dark:bg-orange-600 dark:hover:bg-orange-700",
        icon: AlarmClockIcon,
      },
      red: {
        container:
          "bg-destructive/15 border-destructive/30 dark:bg-red-950/30 dark:border-red-800",
        title: "text-destructive dark:text-red-300 font-semibold",
        message: "text-destructive/80 dark:text-red-400",
        buttonVariant: "destructive" as const,
        buttonClass: "",
        icon: AlarmClockIcon,
      },
    }),
    []
  );

  const { title, message, color } = cardContent;
  const colors = colorClasses[color];

  // Don't render anything if we shouldn't show the card (use hook value)
  if (!shouldShowUpgrade) {
    return null;
  }

  return (
    <div className={cn("border-t p-3", colors.container)}>
      <div className="mb-2 flex flex-col gap-1">
        <h3 className={cn("text-sm", colors.title)}>{title}</h3>
        <p className={cn("text-xs", colors.message)}>{message}</p>
      </div>

      <div className="flex gap-2">
        <Button
          className={cn("w-full", colors.buttonClass)}
          render={<Link preload="intent" to="/app/upgrade" />}
          size="sm"
          variant={colors.buttonVariant}
        >
          <HugeiconsIcon className="size-3" icon={colors.icon} />
          Upgrade
        </Button>
      </div>
    </div>
  );
};

export default TrialUpgradeCard;
