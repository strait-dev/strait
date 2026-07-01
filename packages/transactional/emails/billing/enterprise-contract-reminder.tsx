import { Link } from "@react-email/components";
import {
  BillingButton,
  BillingLayout,
  BillingText,
  salesEmail,
} from "./billing-layout";

type EnterpriseContractReminderProps = {
  contractEndDate: string;
  autoRenew: boolean;
  daysRemaining: number;
};

const EnterpriseContractReminder = ({
  contractEndDate,
  autoRenew,
  daysRemaining,
}: EnterpriseContractReminderProps) => (
  <BillingLayout
    heading={
      autoRenew
        ? "Contract renewal notice"
        : "Enterprise contract expiring soon"
    }
    preview={
      autoRenew
        ? `Enterprise contract renewing in ${daysRemaining} days`
        : `Enterprise contract expiring in ${daysRemaining} days`
    }
  >
    <BillingText>
      {autoRenew
        ? "Your Enterprise contract is set to auto-renew"
        : "Your Enterprise contract expires"}{" "}
      on <strong style={{ color: "#252525" }}>{contractEndDate}</strong> (
      {daysRemaining} days from now).
    </BillingText>

    <br />

    {autoRenew ? (
      <BillingText>
        No action is required. Your contract terms will continue unchanged. If
        you need to modify your contract, contact your Customer Success Manager
        or email{" "}
        <Link
          href={`mailto:${salesEmail}`}
          style={{ color: "#171717", textDecoration: "underline" }}
        >
          {salesEmail}
        </Link>
        .
      </BillingText>
    ) : (
      <>
        <BillingText>
          After expiry, your organization will be moved to the Scale plan. To
          renew your Enterprise contract, contact your Customer Success Manager
          or email{" "}
          <Link
            href={`mailto:${salesEmail}`}
            style={{ color: "#171717", textDecoration: "underline" }}
          >
            {salesEmail}
          </Link>
          .
        </BillingText>

        <br />

        <BillingButton href={`mailto:${salesEmail}`}>
          Contact sales
        </BillingButton>
      </>
    )}
  </BillingLayout>
);

EnterpriseContractReminder.PreviewProps = {
  contractEndDate: "April 1, 2027",
  autoRenew: false,
  daysRemaining: 7,
};

export default EnterpriseContractReminder;
