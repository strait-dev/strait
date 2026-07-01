import {
  BillingButton,
  BillingLayout,
  BillingText,
  upgradeUrl,
} from "./billing-layout";

type DowngradeHTTPJobsWarningProps = {
  periodEnd: string;
  jobCount: number;
};

const DowngradeHTTPJobsWarning = ({
  periodEnd,
  jobCount,
}: DowngradeHTTPJobsWarningProps) => (
  <BillingLayout
    heading="HTTP jobs will be paused"
    preview={`${jobCount} HTTP-mode jobs will be paused on ${periodEnd}`}
  >
    <BillingText>
      Your plan downgrade takes effect on{" "}
      <strong style={{ color: "#252525" }}>{periodEnd}</strong>. At that time,
      your{" "}
      <strong style={{ color: "#252525" }}>{jobCount} HTTP-mode job(s)</strong>{" "}
      will be automatically paused because the new plan does not support HTTP
      execution mode.
    </BillingText>

    <br />

    <BillingText>
      Your job configurations and run history will be fully preserved. To keep
      your HTTP jobs running, upgrade back to Pro or higher before the period
      ends.
    </BillingText>

    <br />

    <BillingButton href={upgradeUrl}>Upgrade plan</BillingButton>
  </BillingLayout>
);

DowngradeHTTPJobsWarning.PreviewProps = {
  periodEnd: "2026-05-01",
  jobCount: 3,
};

export default DowngradeHTTPJobsWarning;
