import {
  MetricRow,
  MetricTable,
  NotificationLayout,
  NotificationText,
} from "./notification-layout";

type CostAnomalyProps = {
  orgId: string;
  severity: string;
  spikeRatio: string;
  todaySpend: string;
  sevenDayAverage: string;
  topContributor: string;
};

const CostAnomaly = ({
  orgId,
  severity,
  spikeRatio,
  todaySpend,
  sevenDayAverage,
  topContributor,
}: CostAnomalyProps) => (
  <NotificationLayout
    heading="Cost Anomaly Detected"
    preview="Cost anomaly detected"
  >
    <NotificationText>
      A <strong style={{ color: "#252525" }}>{severity}</strong>-severity
      spending spike of{" "}
      <strong style={{ color: "#252525" }}>{spikeRatio}</strong> was detected
      for organization <strong style={{ color: "#252525" }}>{orgId}</strong>.
    </NotificationText>

    <br />

    <MetricTable>
      <MetricRow label="Today's spend" value={todaySpend} />
      <MetricRow label="7-day average" value={sevenDayAverage} />
      <MetricRow label="Spike ratio" value={spikeRatio} />
      <MetricRow label="Top contributor" value={topContributor} />
    </MetricTable>

    <br />

    <NotificationText>
      Review your usage to ensure this activity is expected.
    </NotificationText>
  </NotificationLayout>
);

CostAnomaly.PreviewProps = {
  orgId: "org_123",
  severity: "high",
  spikeRatio: "2.4x",
  todaySpend: "1200000 micro-USD",
  sevenDayAverage: "500000 micro-USD",
  topContributor: "project_123",
};

export default CostAnomaly;
