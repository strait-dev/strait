// @ts-nocheck
import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

const PaymentStatusBanner = () => {
  const { data } = useQuery(orgUsageQueryOptions());

  if (!data?.payment_status) {
    return null;
  }

  const isRestricted = data.payment_status === "restricted";
  const isGrace = data.payment_status === "grace";

  if (!(isRestricted || isGrace)) {
    return null;
  }

  const handleUpdatePayment = async () => {
    const result = await getCustomerPortalUrlServerFn();
    if (result.url) {
      window.location.href = result.url;
    }
  };

  const graceEnd = data.grace_period_end
    ? new Date(data.grace_period_end).toLocaleDateString()
    : null;

  if (isRestricted) {
    return (
      <div className="flex items-center justify-between rounded-custom border border-destructive/50 bg-destructive/10 px-4 py-2">
        <p className="text-destructive text-sm">
          Your account is restricted due to failed payment. New runs are
          blocked.
        </p>
        <Button onClick={handleUpdatePayment} size="sm" variant="destructive">
          Update Payment Method
        </Button>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between rounded-custom border border-yellow-200 bg-yellow-50 px-4 py-2 dark:border-yellow-800 dark:bg-yellow-950">
      <p className="text-sm text-yellow-800 dark:text-yellow-200">
        Payment failed.
        {graceEnd
          ? ` Update your payment method by ${graceEnd} to avoid service interruption.`
          : " Please update your payment method."}
      </p>
      <Button onClick={handleUpdatePayment} size="sm" variant="default">
        Update Payment
      </Button>
    </div>
  );
};

export default PaymentStatusBanner;
