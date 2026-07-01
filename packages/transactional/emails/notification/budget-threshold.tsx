import {
  MetricRow,
  MetricTable,
  NotificationLayout,
  NotificationText,
} from "./notification-layout";

type BudgetThresholdProps = {
  projectId: string;
  thresholdPercent: string;
  dailyCost: string;
  budgetLimit: string;
};

const BudgetThreshold = ({
  projectId,
  thresholdPercent,
  dailyCost,
  budgetLimit,
}: BudgetThresholdProps) => (
  <NotificationLayout
    heading="Compute Budget Threshold Reached"
    preview="Compute budget threshold reached"
  >
    <NotificationText>
      Project <strong style={{ color: "#252525" }}>{projectId}</strong> has
      exceeded <strong style={{ color: "#252525" }}>{thresholdPercent}</strong>{" "}
      of its daily compute budget.
    </NotificationText>

    <br />

    <MetricTable>
      <MetricRow label="Daily cost" value={dailyCost} />
      <MetricRow label="Budget limit" value={budgetLimit} />
    </MetricTable>
  </NotificationLayout>
);

BudgetThreshold.PreviewProps = {
  projectId: "project_123",
  thresholdPercent: "80%",
  dailyCost: "800000 micro-USD",
  budgetLimit: "1000000 micro-USD",
};

export default BudgetThreshold;
