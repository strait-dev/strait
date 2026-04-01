import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { cn } from "@strait/ui/utils/index";
import StatusBadge from "@/components/dashboard/status-badge";
import type { RunStatus } from "@/hooks/api/types";
import { formatMicroUsd } from "@/lib/format";

type AgentRunSummary = {
  attempt: number;
  cost_microusd: number;
  duration_secs: number;
  error_class?: string;
  model: string;
  run_id: string;
  status: string;
  tool_call_count: number;
  total_tokens: number;
};

type ToolCallDiff = {
  count_a: number;
  count_b: number;
  tool_name: string;
};

export type AgentRunComparison = {
  cost_diff_microusd: number;
  duration_diff_secs: number;
  model_match: boolean;
  run_a: AgentRunSummary;
  run_b: AgentRunSummary;
  status_match: boolean;
  token_diff: number;
  tool_call_diffs?: ToolCallDiff[];
};

function MetricRow({
  diff,
  format,
  label,
  valueA,
  valueB,
}: {
  diff: number;
  format?: (v: number) => string;
  label: string;
  valueA: number;
  valueB: number;
}) {
  const fmt = format ?? String;
  const diffPrefix = diff > 0 ? "+" : "";

  let diffColor = "text-muted-foreground";
  if (diff > 0) {
    diffColor = "text-red-500";
  }
  if (diff < 0) {
    diffColor = "text-green-500";
  }

  return (
    <div className="grid grid-cols-3 gap-4 border-b py-2 text-sm last:border-b-0">
      <span className="font-medium text-muted-foreground">{label}</span>
      <span className="text-center">{fmt(valueA)}</span>
      <span className="text-center">
        {fmt(valueB)}{" "}
        <span className={cn("text-xs", diffColor)}>
          ({diffPrefix}
          {fmt(diff)})
        </span>
      </span>
    </div>
  );
}

/** Side-by-side comparison of two agent runs with color-coded metric diffs. */
export default function AgentRunComparisonView({
  comparison,
}: {
  comparison: AgentRunComparison;
}) {
  const { run_a, run_b } = comparison;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Run Comparison</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-4 border-b pb-2 font-medium text-muted-foreground text-xs">
            <span>Metric</span>
            <span className="text-center">Run A</span>
            <span className="text-center">Run B</span>
          </div>

          <div className="grid grid-cols-3 gap-4 border-b py-2 text-sm">
            <span className="font-medium text-muted-foreground">Status</span>
            <span className="flex justify-center">
              <StatusBadge showDot status={run_a.status as RunStatus} />
            </span>
            <span className="flex justify-center">
              <StatusBadge showDot status={run_b.status as RunStatus} />
            </span>
          </div>

          <div className="grid grid-cols-3 gap-4 border-b py-2 text-sm">
            <span className="font-medium text-muted-foreground">Model</span>
            <span className="text-center font-mono text-xs">
              {run_a.model || "-"}
            </span>
            <span className="text-center font-mono text-xs">
              {run_b.model || "-"}
              {!comparison.model_match && (
                <Badge className="ml-1 text-[10px]" variant="destructive">
                  different
                </Badge>
              )}
            </span>
          </div>

          <MetricRow
            diff={comparison.cost_diff_microusd}
            format={formatMicroUsd}
            label="Cost"
            valueA={run_a.cost_microusd}
            valueB={run_b.cost_microusd}
          />
          <MetricRow
            diff={comparison.token_diff}
            label="Tokens"
            valueA={Number(run_a.total_tokens)}
            valueB={Number(run_b.total_tokens)}
          />
          <MetricRow
            diff={Number(comparison.duration_diff_secs.toFixed(2))}
            format={(v) => `${v.toFixed(1)}s`}
            label="Duration"
            valueA={run_a.duration_secs}
            valueB={run_b.duration_secs}
          />
          <MetricRow
            diff={run_a.tool_call_count - run_b.tool_call_count}
            label="Tool Calls"
            valueA={run_a.tool_call_count}
            valueB={run_b.tool_call_count}
          />
        </CardContent>
      </Card>

      {comparison.tool_call_diffs && comparison.tool_call_diffs.length > 0 && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Tool Call Differences</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-4 gap-2 border-b pb-1 font-medium text-muted-foreground text-xs">
              <span>Tool</span>
              <span className="text-center">Run A</span>
              <span className="text-center">Run B</span>
              <span className="text-center">Diff</span>
            </div>
            {comparison.tool_call_diffs.map((diff) => (
              <div
                className="grid grid-cols-4 gap-2 border-b py-1.5 text-sm last:border-b-0"
                key={diff.tool_name}
              >
                <span className="font-mono text-xs">{diff.tool_name}</span>
                <span className="text-center">{diff.count_a}</span>
                <span className="text-center">{diff.count_b}</span>
                <span
                  className={cn(
                    "text-center text-xs",
                    diff.count_a - diff.count_b > 0
                      ? "text-red-500"
                      : "text-green-500"
                  )}
                >
                  {diff.count_a - diff.count_b > 0 ? "+" : ""}
                  {diff.count_a - diff.count_b}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {(run_a.error_class || run_b.error_class) && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Error Analysis</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-muted-foreground text-xs">Run A</span>
                <p className="font-mono">{run_a.error_class || "none"}</p>
              </div>
              <div>
                <span className="text-muted-foreground text-xs">Run B</span>
                <p className="font-mono">{run_b.error_class || "none"}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
