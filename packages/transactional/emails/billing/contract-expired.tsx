import {
  BillingButton,
  BillingLayout,
  BillingText,
  salesEmail,
} from "./billing-layout";

type ContractExpiredProps = {
  contractEndDate: string;
};

const ContractExpired = ({ contractEndDate }: ContractExpiredProps) => (
  <BillingLayout
    heading="Enterprise contract expired"
    preview="Your enterprise contract has expired"
  >
    <BillingText>
      Your Enterprise contract expired on{" "}
      <strong style={{ color: "#252525" }}>{contractEndDate}</strong>. Your
      organization has been placed in restricted mode. New job runs will be
      blocked until your contract is renewed.
    </BillingText>

    <br />

    <BillingButton href={`mailto:${salesEmail}`}>
      Contact sales to renew
    </BillingButton>
  </BillingLayout>
);

ContractExpired.PreviewProps = {
  contractEndDate: "2026-01-31",
};

export default ContractExpired;
