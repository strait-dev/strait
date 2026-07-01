import {
  appBillingUrl,
  BillingButton,
  BillingLayout,
  BillingText,
} from "./billing-layout";

type TrialEndingSoonProps = {
  trialEndDate: string;
  daysRemaining: number;
};

const TrialEndingSoon = ({
  trialEndDate,
  daysRemaining,
}: TrialEndingSoonProps) => (
  <BillingLayout
    heading="Temporary access ending soon"
    preview={`Temporary access ends in ${daysRemaining} days`}
  >
    <BillingText>
      Your temporary Strait access ends on{" "}
      <strong style={{ color: "#252525" }}>{trialEndDate}</strong> (
      {daysRemaining} days from now). Choose a launch plan or update billing to
      keep paid-plan limits.
    </BillingText>

    <br />

    <BillingButton href={appBillingUrl}>Manage billing</BillingButton>
  </BillingLayout>
);

TrialEndingSoon.PreviewProps = {
  trialEndDate: "2026-06-30",
  daysRemaining: 5,
};

export default TrialEndingSoon;
