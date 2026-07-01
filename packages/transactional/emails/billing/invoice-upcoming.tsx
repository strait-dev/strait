import {
  appBillingUrl,
  BillingButton,
  BillingLayout,
  BillingText,
} from "./billing-layout";

type InvoiceUpcomingProps = {
  amountDue: string;
  dueDate: string;
};

const InvoiceUpcoming = ({ amountDue, dueDate }: InvoiceUpcomingProps) => (
  <BillingLayout heading="Upcoming invoice" preview="Upcoming invoice">
    <BillingText>
      Your next Strait invoice of{" "}
      <strong style={{ color: "#252525" }}>{amountDue}</strong> will be charged
      on <strong style={{ color: "#252525" }}>{dueDate}</strong>.
    </BillingText>

    <br />

    <BillingButton href={appBillingUrl}>View billing</BillingButton>
  </BillingLayout>
);

InvoiceUpcoming.PreviewProps = {
  amountDue: "$125.00",
  dueDate: "2026-07-01",
};

export default InvoiceUpcoming;
