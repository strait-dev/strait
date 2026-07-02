import { HugeiconsIcon } from "@hugeicons/react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Frame, FramePanel } from "@strait/ui/components/frame";
import { toast } from "@strait/ui/components/toast";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import type { SubscriptionStateData } from "@/hooks/subscription/subscription-state";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { getPostHog } from "@/lib/analytics";
import {
  AlarmClockIcon,
  AlertCircleIcon,
  CheckCircleIcon,
  CreditCardIcon,
  LinkSquareIcon,
  SparklesIcon,
} from "@/lib/icons";
import { captureException } from "@/lib/sentry";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

const ATTENTION_STATUSES = new Set(["incomplete", "past_due", "unpaid"]);

const PLAN_NAMES = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  enterprise: "Enterprise",
} as const;

const INTERVAL_NAMES = {
  monthly: "Monthly",
  yearly: "Annual",
  month: "Monthly",
  year: "Annual",
} as const;

function getSubscriptionStatusInfo(status?: string) {
  switch (status) {
    case "active":
      return {
        message: "Active",
        icon: CheckCircleIcon,
        variant: "success" as const,
      };
    case "canceled":
      return {
        message: "Canceled",
        icon: AlertCircleIcon,
        variant: "destructive" as const,
      };
    case "incomplete":
    case "past_due":
    case "unpaid":
      return {
        message: "Needs attention",
        icon: AlarmClockIcon,
        variant: "destructive" as const,
      };
    default:
      return {
        message: "Inactive",
        icon: null,
        variant: "secondary" as const,
      };
  }
}

function getSubscriptionStatusMessage({
  currentPeriodEnd,
  isActive,
  isCanceled,
  needsAttention,
}: {
  currentPeriodEnd?: Date | string | null;
  isActive: boolean;
  isCanceled: boolean;
  needsAttention: boolean;
}) {
  if (isActive && currentPeriodEnd) {
    const nextBillingDate = new Date(currentPeriodEnd).toLocaleDateString(
      "en-US",
      {
        day: "numeric",
        month: "short",
        year: "numeric",
      }
    );
    return isCanceled
      ? `Cancels on ${nextBillingDate}`
      : `Next billing on ${nextBillingDate}`;
  }
  if (isCanceled) {
    return "Your plan has been canceled. Choose a new plan to continue using Strait.";
  }
  if (needsAttention) {
    return "Your subscription needs attention. Please update your payment method.";
  }
  return isActive ? "Active subscription" : "No active subscription";
}

function getPlanInfo(data: SubscriptionStateData) {
  const { subscription, isActive, needsAttention, isCanceled, plan } = data;
  const statusInfo = getSubscriptionStatusInfo(subscription?.status);
  const planName = PLAN_NAMES[plan as keyof typeof PLAN_NAMES] || "Unknown";
  const intervalValue = subscription?.recurringInterval ?? "monthly";
  const intervalName =
    INTERVAL_NAMES[intervalValue as keyof typeof INTERVAL_NAMES] || "Monthly";

  return {
    planName,
    intervalName,
    statusInfo,
    statusMessage: getSubscriptionStatusMessage({
      currentPeriodEnd: subscription?.currentPeriodEnd,
      isActive,
      isCanceled,
      needsAttention,
    }),
    isActive: isActive || subscription?.status === "active",
    isCanceled: isCanceled || subscription?.status === "canceled",
    needsAttention:
      needsAttention ||
      ATTENTION_STATUSES.has((subscription?.status as string) ?? ""),
  };
}

const SubscriptionOverview = () => {
  const [isLoading, setIsLoading] = useState<string | null>(null);

  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { subscription } = data;

  // Helper to open portal
  const openPortal = async (loadingKey: string) => {
    getPostHog()?.capture("billing_portal_opened", { action: loadingKey });
    setIsLoading(loadingKey);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        setIsLoading(null);
        return;
      }
      window.location.assign(result.url);
    } catch (error) {
      captureException(error);
      toast.error("Failed to open customer portal");
    }
    setIsLoading(null);
  };

  // Memoize handlers
  const handleOpenPortal = async () => {
    await openPortal("portal");
  };

  const handleCancelSubscription = async () => {
    await openPortal("cancel");
  };

  const handleReactivateSubscription = async () => {
    await openPortal("reactivate");
  };

  const planInfo = getPlanInfo(data);

  // No subscription case
  if (!subscription) {
    return (
      <div className="space-y-6">
        <Card variant="outline">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon className="size-5" icon={CreditCardIcon} />
              No Active Subscription
            </CardTitle>
            <CardDescription>
              You don't have an active subscription. Choose a plan to start
              using Strait.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button render={<Link preload="intent" to="/app/upgrade" />}>
              Choose plan
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Plan Status Overview */}
      <Card variant="outline">
        <CardHeader>
          <div className="flex items-start justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <HugeiconsIcon className="size-5" icon={CreditCardIcon} />
                {planInfo.planName} Plan
              </CardTitle>
              <CardDescription className="mt-1">
                {planInfo.statusMessage}
              </CardDescription>
            </div>
            <Badge
              iconLeft={planInfo.statusInfo.icon ?? undefined}
              variant={
                `${planInfo.statusInfo.variant}-light` as BadgeProps["variant"]
              }
            >
              {planInfo.statusInfo.message}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <Frame>
            <FramePanel>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-1">
                  <div className="font-medium text-muted-foreground text-sm">
                    Billing cycle
                  </div>
                  <p className="font-normal">{planInfo.intervalName}</p>
                </div>

                {subscription?.currentPeriodEnd ? (
                  <div className="space-y-1">
                    <div className="font-medium text-muted-foreground text-sm">
                      {planInfo.isCanceled ? "Cancels on" : "Next billing"}
                    </div>
                    <p className="font-normal">
                      {new Date(
                        subscription.currentPeriodEnd
                      ).toLocaleDateString("en-US", {
                        day: "numeric",
                        month: "short",
                        year: "numeric",
                      })}
                    </p>
                  </div>
                ) : null}
              </div>
            </FramePanel>
          </Frame>
        </CardContent>
      </Card>

      {/* Subscription Management Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Manage your subscription</CardTitle>
          <CardDescription>
            Access the customer portal to manage your subscription, payment
            methods, and view invoices.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-3 sm:flex-row">
            <Button
              className="flex items-center gap-2"
              disabled={isLoading === "portal"}
              onClick={handleOpenPortal}
            >
              <HugeiconsIcon className="size-4" icon={LinkSquareIcon} />
              {isLoading === "portal" ? "Opening..." : "Customer portal"}
            </Button>

            <Button
              render={
                <Link className="flex items-center gap-2" to="/app/upgrade" />
              }
              variant="outline"
            >
              <HugeiconsIcon className="size-4" icon={SparklesIcon} />
              {planInfo.isActive ? "Change plan" : "Choose plan"}
            </Button>

            {planInfo.isActive && planInfo.isCanceled ? (
              <Button
                disabled={isLoading === "reactivate"}
                onClick={handleReactivateSubscription}
                variant="outline"
              >
                {isLoading === "reactivate" ? "Reactivating..." : "Reactivate"}
              </Button>
            ) : null}
            {planInfo.isActive && !planInfo.isCanceled ? (
              <Button
                disabled={isLoading === "cancel"}
                onClick={handleCancelSubscription}
                variant="destructive"
              >
                {isLoading === "cancel" ? "Canceling..." : "Cancel"}
              </Button>
            ) : null}
          </div>

          <Alert>
            <HugeiconsIcon className="size-4" icon={AlertCircleIcon} />
            <AlertTitle>Customer portal</AlertTitle>
            <AlertDescription>
              Update payment methods, download invoices, view payment history,
              and manage your subscription details.
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    </div>
  );
};

export default SubscriptionOverview;
