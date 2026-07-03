import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { NoticeBanner } from "@strait/ui/components/notice-banner";
import { toast } from "@strait/ui/components/toast";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { CreditCardIcon, LinkSquareIcon } from "@/lib/icons";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

const PaymentPendingCard = () => {
  const [isLoading, setIsLoading] = useState(false);
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { hasPendingPayment } = data;

  const handleOpenPortal = async () => {
    setIsLoading(true);

    try {
      // Use custom server function that looks up customer by email
      const result = await getCustomerPortalUrlServerFn();

      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        setIsLoading(false);
        return;
      }

      // Redirect to the portal URL
      window.location.assign(result.url);
    } catch {
      toast.error("Failed to open customer portal");
    }
    setIsLoading(false);
  };

  // Don't render if there are no payment issues
  if (!hasPendingPayment) {
    return null;
  }

  return (
    <NoticeBanner
      action={
        <div className="flex gap-2">
          <Button
            disabled={isLoading}
            onClick={handleOpenPortal}
            size="sm"
            variant="warning-solid"
          >
            <HugeiconsIcon className="size-3" icon={LinkSquareIcon} />
            {isLoading ? "Opening..." : "Manage"}
          </Button>

          <Button
            className="flex-1"
            render={<Link preload="intent" to="/app/settings" />}
            size="sm"
            variant="outline"
          >
            <HugeiconsIcon className="size-3" icon={CreditCardIcon} />
            View details
          </Button>
        </div>
      }
      title="Payment pending"
      variant="warning"
    >
      You have a pending payment. Complete the payment to continue using Strait.
    </NoticeBanner>
  );
};

export default PaymentPendingCard;
