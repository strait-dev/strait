import { BillingLayout, BillingText } from "./billing-layout";

type DisputeAlertProps = {
  disputeAmount: string;
};

const DisputeAlert = ({ disputeAmount }: DisputeAlertProps) => (
  <BillingLayout
    heading="Payment dispute received"
    preview="Payment dispute received"
  >
    <BillingText>
      A payment dispute of{" "}
      <strong style={{ color: "#252525" }}>{disputeAmount}</strong> has been
      opened on your account. Your service will continue while the dispute is
      under review.
    </BillingText>

    <br />

    <BillingText>
      If this dispute is not resolved, your account may be restricted. Please
      check your email for details from your payment provider.
    </BillingText>
  </BillingLayout>
);

DisputeAlert.PreviewProps = {
  disputeAmount: "$25.00",
};

export default DisputeAlert;
