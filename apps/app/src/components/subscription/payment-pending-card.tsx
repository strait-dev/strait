import { CreditCardIcon, LinkSquare01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { toast } from "@strait/ui/components/toast/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

const PaymentPendingCard = () => {
  const [isLoading, setIsLoading] = useState(false);
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { hasPendingPayment } = data;

  const handleOpenPortal = useCallback(async () => {
    setIsLoading(true);

    try {
      // Use custom server function that looks up customer by email
      const result = await getCustomerPortalUrlServerFn();

      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        return;
      }

      // Redirect to the portal URL
      window.location.href = result.url;
    } catch {
      toast.error("Failed to open customer portal");
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Don't render if there are no payment issues
  if (!hasPendingPayment) {
    return null;
  }

  return (
    <div className="rounded-b-custom border-sidebar-border border-t bg-sidebar-accent p-3 text-sidebar-foreground">
      <div className="mb-2 flex flex-col gap-1">
        <h3 className="font-medium text-sm">Payment pending</h3>
        <p className="text-sidebar-foreground/70 text-xs">
          You have a pending payment. Complete the payment to continue using
          Strait.
        </p>
      </div>

      <div className="flex gap-2">
        <Button
          className="flex-1"
          disabled={isLoading}
          onClick={handleOpenPortal}
          size="sm"
        >
          <HugeiconsIcon className="size-3" icon={LinkSquare01Icon} />
          {isLoading ? "Opening..." : "Manage"}
        </Button>

        <Button
          className="flex-1"
          render={<Link preload="intent" to="/app/settings" />}
          size="sm"
          variant="outline"
        >
          <HugeiconsIcon className="size-3" icon={CreditCardIcon} />
          View Details
        </Button>
      </div>
    </div>
  );
};

export default PaymentPendingCard;
