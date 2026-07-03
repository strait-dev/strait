import { Button } from "@strait/ui/components/button";
import {
  NoticeBanner,
  NoticeBannerAction,
} from "@strait/ui/components/notice-banner";
import { useQuery } from "@tanstack/react-query";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

const handleUpdatePayment = async () => {
  const result = await getCustomerPortalUrlServerFn();
  if (result.url) {
    window.location.href = result.url;
  }
};

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

  const graceEnd = data.grace_period_end
    ? new Date(data.grace_period_end).toLocaleDateString()
    : null;

  if (isRestricted) {
    return (
      <NoticeBanner
        action={
          <NoticeBannerAction>
            <Button onClick={handleUpdatePayment} variant="destructive">
              Update payment method
            </Button>
          </NoticeBannerAction>
        }
        title="Account restricted"
        variant="destructive"
      >
        Your account is restricted due to failed payment. New runs are blocked.
      </NoticeBanner>
    );
  }

  return (
    <NoticeBanner
      action={
        <NoticeBannerAction>
          <Button onClick={handleUpdatePayment}>Update payment</Button>
        </NoticeBannerAction>
      }
      title="Payment failed"
      variant="warning"
    >
      Payment failed.
      {graceEnd
        ? ` Update your payment method by ${graceEnd} to avoid service interruption.`
        : " Please update your payment method."}
    </NoticeBanner>
  );
};

export default PaymentStatusBanner;
