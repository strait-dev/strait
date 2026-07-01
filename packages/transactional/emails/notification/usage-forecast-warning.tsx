import {
  MetricRow,
  MetricTable,
  NotificationLayout,
  NotificationText,
} from "./notification-layout";

type UsageForecastWarningProps = {
  orgId: string;
  daysUntilLimit: number;
  recommendedPlan: string;
  projectedRuns: string;
};

const UsageForecastWarning = ({
  orgId,
  daysUntilLimit,
  recommendedPlan,
  projectedRuns,
}: UsageForecastWarningProps) => (
  <NotificationLayout
    heading="Usage forecast warning"
    preview="Projected usage will reach a plan limit soon"
  >
    <NotificationText>
      Organization <strong style={{ color: "#252525" }}>{orgId}</strong> is
      projected to reach a plan limit in{" "}
      <strong style={{ color: "#252525" }}>{daysUntilLimit} day(s)</strong>.
    </NotificationText>

    <br />

    <MetricTable>
      <MetricRow label="Recommended plan" value={recommendedPlan} />
      <MetricRow label="Projected monthly runs" value={projectedRuns} />
    </MetricTable>
  </NotificationLayout>
);

UsageForecastWarning.PreviewProps = {
  orgId: "org_123",
  daysUntilLimit: 3,
  recommendedPlan: "Scale",
  projectedRuns: "5,500,000",
};

export default UsageForecastWarning;
