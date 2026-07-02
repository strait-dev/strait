import {
  MetricRow,
  MetricTable,
  NotificationLayout,
  NotificationText,
} from "./notification-layout";

type NotificationSpendingLimitWarningProps = {
  orgId: string;
  overagePercent: string;
  currentSpend: string;
  spendingLimit: string;
};

const NotificationSpendingLimitWarning = ({
  orgId,
  overagePercent,
  currentSpend,
  spendingLimit,
}: NotificationSpendingLimitWarningProps) => (
  <NotificationLayout
    heading="Spending Limit Warning"
    preview="Spending limit warning"
  >
    <NotificationText>
      Your organization <strong style={{ color: "#252525" }}>{orgId}</strong>{" "}
      has reached <strong style={{ color: "#252525" }}>{overagePercent}</strong>{" "}
      of its monthly spending limit.
    </NotificationText>

    <br />

    <MetricTable>
      <MetricRow label="Current spend" value={currentSpend} />
      <MetricRow label="Spending limit" value={spendingLimit} />
    </MetricTable>

    <br />

    <NotificationText>
      Consider adjusting your spending limit or reviewing resource usage.
    </NotificationText>
  </NotificationLayout>
);

NotificationSpendingLimitWarning.PreviewProps = {
  orgId: "org_123",
  overagePercent: "80%",
  currentSpend: "$80.00",
  spendingLimit: "$100.00",
};

export default NotificationSpendingLimitWarning;
