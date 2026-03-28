import { HugeiconsIcon } from "@hugeicons/react";
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
import { toast } from "@strait/ui/components/toast/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useCallback, useMemo, useState } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import {
  AlarmClockIcon,
  AlertCircleIcon,
  CheckCircleIcon,
  CreditCardIcon,
  LinkSquareIcon,
  SparklesIcon,
} from "@/lib/icons";
import { getPostHog } from "@/lib/analytics";
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

const SubscriptionOverview = () => {
  const [isLoading, setIsLoading] = useState<string | null>(null);

  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { subscription, isActive, needsAttention, isCanceled, plan } = data;

  // Helper to open portal
  const openPortal = useCallback(async (loadingKey: string) => {
    getPostHog()?.capture("billing_portal_opened", { action: loadingKey });
    setIsLoading(loadingKey);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        return;
      }
      window.location.href = result.url;
    } catch (error) {
      captureException(error);
      toast.error("Failed to open customer portal");
    } finally {
      setIsLoading(null);
    }
  }, []);

  // Memoize handlers
  const handleOpenPortal = useCallback(async () => {
    await openPortal("portal");
  }, [openPortal]);

  const handleCancelSubscription = useCallback(async () => {
    await openPortal("cancel");
  }, [openPortal]);

  const handleReactivateSubscription = useCallback(async () => {
    await openPortal("reactivate");
  }, [openPortal]);

  // Helper functions to reduce complexity
  const getStatusInfo = useCallback(() => {
    switch (subscription?.status) {
      case "active":
        return {
          message: "Active",
          icon: <HugeiconsIcon className="size-4" icon={CheckCircleIcon} />,
          variant: "success" as const,
          bgGradient: "from-primary/30 to-primary/10",
        };
      case "canceled":
        return {
          message: "Canceled",
          icon: <HugeiconsIcon className="size-4" icon={AlertCircleIcon} />,
          variant: "destructive" as const,
          bgGradient: "from-destructive/30 to-destructive/10",
        };
      case "incomplete":
      case "past_due":
      case "unpaid":
        return {
          message: "Needs Attention",
          icon: <HugeiconsIcon className="size-4" icon={AlarmClockIcon} />,
          variant: "destructive" as const,
          bgGradient: "from-destructive/30 to-destructive/10",
        };
      default:
        return {
          message: "Inactive",
          icon: null,
          variant: "secondary" as const,
          bgGradient: "from-muted-foreground/30 to-muted-foreground/10",
        };
    }
  }, [subscription?.status]);

  const getStatusMessage = useCallback(() => {
    if (isActive && subscription?.currentPeriodEnd) {
      const nextBillingDate = new Date(
        subscription.currentPeriodEnd
      ).toLocaleDateString("en-US", {
        day: "numeric",
        month: "short",
        year: "numeric",
      });
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
  }, [isActive, subscription?.currentPeriodEnd, isCanceled, needsAttention]);

  // Memoize status and plan calculations
  const planInfo = useMemo(() => {
    const statusInfo = getStatusInfo();
    const planName = PLAN_NAMES[plan as keyof typeof PLAN_NAMES] || "Unknown";
    const intervalValue = subscription?.recurringInterval ?? "monthly";
    const intervalName =
      INTERVAL_NAMES[intervalValue as keyof typeof INTERVAL_NAMES] || "Monthly";

    return {
      planName,
      intervalName,
      statusInfo,
      statusMessage: getStatusMessage(),
      isActive: isActive || subscription?.status === "active",
      isCanceled: isCanceled || subscription?.status === "canceled",
      needsAttention:
        needsAttention ||
        ATTENTION_STATUSES.has((subscription?.status as string) ?? ""),
    };
  }, [
    subscription,
    isActive,
    isCanceled,
    needsAttention,
    getStatusInfo,
    getStatusMessage,
    plan,
  ]);

  // No subscription case
  if (!subscription) {
    return (
      <div className="space-y-6">
        <Card className="overflow-hidden border shadow-sm">
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
            <Button
              render={<Link preload="intent" to="/app/upgrade" />}
              size="lg"
            >
              Choose Plan
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Plan Status Overview */}
      <Card className="overflow-hidden border shadow-sm">
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
              className="flex items-center gap-1"
              variant={
                `${planInfo.statusInfo.variant}-light` as BadgeProps["variant"]
              }
            >
              {planInfo.statusInfo.icon}
              {planInfo.statusInfo.message}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="rounded-lg bg-muted/50 p-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-1">
                <div className="font-medium text-muted-foreground text-sm">
                  Billing Cycle
                </div>
                <p className="font-normal">{planInfo.intervalName}</p>
              </div>

              {subscription?.currentPeriodEnd ? (
                <div className="space-y-1">
                  <div className="font-medium text-muted-foreground text-sm">
                    {planInfo.isCanceled ? "Cancels On" : "Next Billing"}
                  </div>
                  <p className="font-normal">
                    {new Date(subscription.currentPeriodEnd).toLocaleDateString(
                      "en-US",
                      {
                        day: "numeric",
                        month: "short",
                        year: "numeric",
                      }
                    )}
                  </p>
                </div>
              ) : null}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Subscription Management Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Manage Your Subscription</CardTitle>
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
              size="lg"
            >
              <HugeiconsIcon className="size-4" icon={LinkSquareIcon} />
              {isLoading === "portal" ? "Opening..." : "Customer Portal"}
            </Button>

            <Button
              render={
                <Link className="flex items-center gap-2" to="/app/upgrade" />
              }
              size="lg"
              variant="outline"
            >
              <HugeiconsIcon className="size-4" icon={SparklesIcon} />
              {planInfo.isActive ? "Change Plan" : "Choose Plan"}
            </Button>

            {!!planInfo.isActive && !!planInfo.isCanceled ? (
              <Button
                disabled={isLoading === "reactivate"}
                onClick={handleReactivateSubscription}
                size="lg"
                variant="outline"
              >
                {isLoading === "reactivate" ? "Reactivating..." : "Reactivate"}
              </Button>
            ) : null}
            {!!planInfo.isActive && !planInfo.isCanceled ? (
              <Button
                className="text-destructive hover:text-destructive"
                disabled={isLoading === "cancel"}
                onClick={handleCancelSubscription}
                size="lg"
                variant="ghost"
              >
                {isLoading === "cancel" ? "Canceling..." : "Cancel"}
              </Button>
            ) : null}
          </div>

          <div className="rounded-lg border bg-muted/30 p-4">
            <div className="flex gap-3">
              <HugeiconsIcon
                className="mt-0.5 size-5 shrink-0 text-muted-foreground"
                icon={AlertCircleIcon}
              />
              <div className="space-y-1">
                <p className="font-medium text-sm">Customer Portal</p>
                <p className="text-muted-foreground text-sm">
                  Update payment methods, download invoices, view payment
                  history, and manage your subscription details.
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default SubscriptionOverview;
