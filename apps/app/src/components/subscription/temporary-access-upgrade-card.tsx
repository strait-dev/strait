import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { NoticeBanner } from "@strait/ui/components/notice-banner";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { AlarmClockIcon, SparklesIcon } from "@/lib/icons";

const TEMPORARY_ACCESS_CRITICAL_DAYS = 2;
const TEMPORARY_ACCESS_WARNING_DAYS = 5;

const TemporaryAccessUpgradeCard = () => {
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { subscription, shouldShowUpgrade, isTrialing, trialDaysLeft } = data;

  const temporaryAccessContent = useMemo(() => {
    if (trialDaysLeft === null || trialDaysLeft <= 0) {
      return {
        title: "Temporary access active",
        message:
          "Choose a launch plan to keep your paid-plan limits after this period ends.",
        color: "green" as const,
      };
    }

    const daysText = trialDaysLeft === 1 ? "1 day" : `${trialDaysLeft} days`;
    const accessMessage = `${daysText} remaining. Choose a launch plan to keep your paid-plan limits.`;

    if (trialDaysLeft <= TEMPORARY_ACCESS_CRITICAL_DAYS) {
      return {
        title:
          trialDaysLeft === 1
            ? "Temporary access ends tomorrow"
            : "Temporary access ending very soon",
        message: accessMessage,
        color: "red" as const,
      };
    }

    if (trialDaysLeft <= TEMPORARY_ACCESS_WARNING_DAYS) {
      return {
        title: "Temporary access ending soon",
        message: accessMessage,
        color: "orange" as const,
      };
    }

    return {
      title: "Temporary access active",
      message: accessMessage,
      color: "green" as const,
    };
  }, [trialDaysLeft]);

  const subscriptionContent = useMemo(() => {
    if (!subscription) {
      return {
        title: "You don't have an active subscription",
        message: "Choose a paid plan when you need higher launch limits.",
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

  const cardContent = useMemo(() => {
    if (isTrialing || subscription?.status === "trialing") {
      return temporaryAccessContent;
    }
    return subscriptionContent;
  }, [subscription, isTrialing, temporaryAccessContent, subscriptionContent]);

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

export default TemporaryAccessUpgradeCard;
