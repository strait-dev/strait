import { BillingLayout, BillingText } from "./billing-layout";

type DunningStepProps = {
  planName: string;
  step: number;
};

const copyForStep = (planName: string, step: number) => {
  switch (step) {
    case 1:
      return {
        heading: "Payment failed",
        preview: "Payment failed - action required",
        body: (
          <>
            We could not collect payment for your{" "}
            <strong style={{ color: "#252525" }}>{planName}</strong> plan.
            Update your billing details to avoid service disruption.
          </>
        ),
      };
    case 2:
      return {
        heading: "Payment past due",
        preview: "Payment still past due (day 3)",
        body: (
          <>
            Three days have passed without a successful payment on your{" "}
            <strong style={{ color: "#252525" }}>{planName}</strong> plan.
            Please update your billing details.
          </>
        ),
      };
    case 3:
      return {
        heading: "Payment past due",
        preview: "Payment still past due (day 7)",
        body: (
          <>
            Your <strong style={{ color: "#252525" }}>{planName}</strong> plan
            is one week past due. Access will be restricted in seven more days
            if payment is not received.
          </>
        ),
      };
    case 4:
      return {
        heading: "Access restricted",
        preview: "Access restricted - payment required",
        body: (
          <>
            Your <strong style={{ color: "#252525" }}>{planName}</strong> plan
            has entered restricted mode after 14 days without payment. New runs
            are blocked until your invoice is paid.
          </>
        ),
      };
    case 5:
      return {
        heading: "Final notice",
        preview: "Final notice before suspension",
        body: (
          <>
            This is the final notice for your{" "}
            <strong style={{ color: "#252525" }}>{planName}</strong> plan. The
            subscription will be suspended in 30 days if no payment is received.
          </>
        ),
      };
    case 6:
      return {
        heading: "Subscription suspended",
        preview: "Subscription suspended",
        body: (
          <>
            Your <strong style={{ color: "#252525" }}>{planName}</strong>{" "}
            subscription has been suspended. Contact support to reactivate.
          </>
        ),
      };
    default:
      return {
        heading: "Payment past due",
        preview: "Payment past due",
        body: (
          <>
            Your <strong style={{ color: "#252525" }}>{planName}</strong> plan
            has an outstanding balance. Please update your billing details.
          </>
        ),
      };
  }
};

const DunningStep = ({ planName, step }: DunningStepProps) => {
  const copy = copyForStep(planName, step);

  return (
    <BillingLayout heading={copy.heading} preview={copy.preview}>
      <BillingText>{copy.body}</BillingText>
    </BillingLayout>
  );
};

DunningStep.PreviewProps = {
  planName: "Business",
  step: 4,
};

export default DunningStep;
