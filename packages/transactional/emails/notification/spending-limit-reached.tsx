import {
  MetricRow,
  MetricTable,
  NotificationLayout,
  NotificationText,
} from "./notification-layout";

type NotificationSpendingLimitReachedProps = {
  orgId: string;
  currentSpend: string;
  spendingLimit: string;
};

const NotificationSpendingLimitReached = ({
  orgId,
  currentSpend,
  spendingLimit,
}: NotificationSpendingLimitReachedProps) => (
  <NotificationLayout
    heading="Spending Limit Reached"
    preview="Spending limit reached"
  >
    <NotificationText>
      Your organization <strong style={{ color: "#252525" }}>{orgId}</strong>{" "}
      has reached its monthly spending limit of{" "}
      <strong style={{ color: "#252525" }}>{spendingLimit}</strong>.
    </NotificationText>

    <br />

    <MetricTable>
      <MetricRow label="Current spend" value={currentSpend} />
      <MetricRow label="Spending limit" value={spendingLimit} />
    </MetricTable>

    <br />

    <NotificationText>
      New runs may be rejected until the next billing period. Increase your
      spending limit to continue.
    </NotificationText>
  </NotificationLayout>
);

NotificationSpendingLimitReached.PreviewProps = {
  orgId: "org_123",
  currentSpend: "$100.00",
  spendingLimit: "$100.00",
};

export default NotificationSpendingLimitReached;
