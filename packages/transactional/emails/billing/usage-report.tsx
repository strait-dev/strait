import { Link } from "@react-email/components";
import { appBillingUrl, BillingLayout, BillingText } from "./billing-layout";

type UsageReportProps = {
  orgId: string;
  planTier: string;
  periodStart: string;
  periodEnd: string;
  addonCount?: number;
  overageAmount?: string;
};

const UsageReport = ({
  orgId,
  planTier,
  periodStart,
  periodEnd,
  addonCount = 0,
  overageAmount,
}: UsageReportProps) => (
  <BillingLayout
    heading="Monthly Usage Report"
    preview="Your Strait usage report"
  >
    <BillingText>
      Here is your usage summary for{" "}
      <strong style={{ color: "#252525" }}>{orgId}</strong> ({planTier} plan).
    </BillingText>

    <br />

    <BillingText>
      Period: {periodStart} to {periodEnd}
    </BillingText>

    <br />

    <BillingText>
      <strong style={{ color: "#252525" }}>Included allowance:</strong> metered
      orchestration runs for this billing period.
    </BillingText>

    {addonCount > 0 ? (
      <>
        <br />
        <BillingText>
          <strong style={{ color: "#252525" }}>Active add-ons:</strong>{" "}
          {addonCount} pack(s)
        </BillingText>
      </>
    ) : null}

    {overageAmount ? (
      <>
        <br />
        <BillingText>
          <strong style={{ color: "#252525" }}>Overage:</strong> {overageAmount}{" "}
          beyond the included run allowance
        </BillingText>
      </>
    ) : null}

    <br />

    <BillingText>Your detailed usage report is attached as a PDF.</BillingText>

    <br />

    <BillingText>
      To manage your billing and spending limits, visit your{" "}
      <Link
        href={appBillingUrl}
        style={{ color: "#171717", textDecoration: "underline" }}
      >
        billing settings
      </Link>
      .
    </BillingText>
  </BillingLayout>
);

UsageReport.PreviewProps = {
  orgId: "org_123",
  planTier: "pro",
  periodStart: "Jun 1",
  periodEnd: "Jun 30, 2026",
  addonCount: 2,
  overageAmount: "$25.00",
};

export default UsageReport;
