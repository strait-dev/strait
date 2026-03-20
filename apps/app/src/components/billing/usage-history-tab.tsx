import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { formatMicroUsd } from "@/lib/format";
import { ActivityIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartEmptyState } from "../dashboard/chart-empty-state";
import { ChartTooltip } from "../dashboard/chart-tooltip";

const LABEL_MAP = {
  compute_cost_microusd: {
    label: "Compute",
    color: CHART_COLORS.active,
    format: formatMicroUsd,
  },
  ai_cost_microusd: {
    label: "AI Cost",
    color: CHART_COLORS.warning,
    format: formatMicroUsd,
  },
};

export function UsageHistoryTab() {
  const { data: history } = useQuery(usageHistoryQueryOptions());

  const isEmpty = !history || history.length === 0;

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="font-medium text-sm">Daily Usage</CardTitle>
          {!isEmpty && (
            <div className="flex items-center gap-1">
              <div className="flex items-center gap-1.5 rounded-md px-2 py-1 text-muted-foreground">
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ backgroundColor: CHART_COLORS.active }}
                />
                <span>Compute</span>
              </div>
              <div className="flex items-center gap-1.5 rounded-md px-2 py-1 text-muted-foreground">
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ backgroundColor: CHART_COLORS.warning }}
                />
                <span>AI Cost</span>
              </div>
            </div>
          )}
        </CardHeader>
        <CardContent>
          <div className="h-[280px]">
            {isEmpty ? (
              <ChartEmptyState
                icon={ActivityIcon}
                message="No usage data yet for this billing period."
              />
            ) : (
              <ResponsiveContainer
                height="100%"
                minHeight={1}
                minWidth={1}
                width="100%"
              >
                <BarChart data={history}>
                  <CartesianGrid
                    className="stroke-border"
                    strokeDasharray="3 3"
                  />
                  <XAxis
                    className="text-muted-foreground"
                    dataKey="date"
                    tick={{ fontSize: 12 }}
                    tickFormatter={(v: string) => v.slice(5)}
                  />
                  <YAxis
                    className="text-muted-foreground"
                    tick={{ fontSize: 12 }}
                    tickFormatter={(v: number) => formatMicroUsd(v)}
                  />
                  <Tooltip
                    content={<ChartTooltip labelMap={LABEL_MAP} />}
                    cursor={{ fill: "var(--muted)" }}
                  />
                  <Bar
                    dataKey="compute_cost_microusd"
                    fill={CHART_COLORS.active}
                    radius={[2, 2, 0, 0]}
                    stackId="cost"
                  />
                  <Bar
                    dataKey="ai_cost_microusd"
                    fill={CHART_COLORS.warning}
                    radius={[2, 2, 0, 0]}
                    stackId="cost"
                  />
                </BarChart>
              </ResponsiveContainer>
            )}
          </div>
        </CardContent>
      </Card>

      {!isEmpty && (
        <Card>
          <CardHeader>
            <CardTitle className="font-medium text-sm">
              Usage Breakdown
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-right">Runs</TableHead>
                  <TableHead className="text-right">Compute Cost</TableHead>
                  <TableHead className="text-right">AI Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((entry) => (
                  <TableRow key={entry.date}>
                    <TableCell>{entry.date}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {entry.runs_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(entry.compute_cost_microusd)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(entry.ai_cost_microusd)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
