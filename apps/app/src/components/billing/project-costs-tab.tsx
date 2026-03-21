import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Progress } from "@strait/ui/components/progress";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { formatMicroUsd } from "@/lib/format";
import { CHART_COLORS } from "@/lib/status-colors";
import ChartTooltip from "../dashboard/chart-tooltip";
import MetricsCard from "./metrics-card";
import ProjectBudgetDialog from "./project-budget-dialog";

type BudgetDialogState = {
  projectId: string;
  projectName: string;
  budgetMicro: number;
  action: string;
} | null;

const ProjectCostsTab = () => {
  const { data: costs } = useQuery(projectCostsQueryOptions());
  const [budgetDialog, setBudgetDialog] = useState<BudgetDialogState>(null);

  const isEmpty = !costs || costs.length === 0;

  const totals = (costs ?? []).reduce(
    (acc, c) => ({
      runs: acc.runs + c.runs,
      compute: acc.compute + c.compute_microusd,
      ai: acc.ai + c.ai_microusd,
      total: acc.total + c.total_microusd,
    }),
    { runs: 0, compute: 0, ai: 0, total: 0 }
  );

  if (isEmpty) {
    return (
      <Card>
        <CardContent className="flex h-48 items-center justify-center">
          <p className="text-muted-foreground text-sm">
            No project cost data for this billing period.
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricsCard label="Total Runs" value={totals.runs.toLocaleString()} />
        <MetricsCard
          label="Compute Cost"
          value={formatMicroUsd(totals.compute)}
        />
        <MetricsCard label="AI Cost" value={formatMicroUsd(totals.ai)} />
        <MetricsCard label="Total Cost" value={formatMicroUsd(totals.total)} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">Cost by Project</CardTitle>
        </CardHeader>
        <CardContent>
          <div
            style={{
              height: `${Math.max(200, costs.length * 36)}px`,
            }}
          >
            <ResponsiveContainer
              height="100%"
              minHeight={1}
              minWidth={1}
              width="100%"
            >
              <BarChart
                data={[...costs].sort(
                  (a, b) => b.total_microusd - a.total_microusd
                )}
                layout="vertical"
              >
                <CartesianGrid
                  className="stroke-border"
                  strokeDasharray="3 3"
                />
                <XAxis
                  className="text-muted-foreground"
                  tick={{ fontSize: 12 }}
                  tickFormatter={(v: number) => formatMicroUsd(v)}
                  type="number"
                />
                <YAxis
                  className="text-muted-foreground"
                  dataKey="name"
                  tick={{ fontSize: 12 }}
                  type="category"
                  width={100}
                />
                <Tooltip
                  content={
                    <ChartTooltip
                      labelMap={{
                        total_microusd: {
                          label: "Total Cost",
                          color: CHART_COLORS.active,
                          format: formatMicroUsd,
                        },
                      }}
                    />
                  }
                  cursor={{ fill: "var(--muted)" }}
                />
                <Bar
                  dataKey="total_microusd"
                  fill={CHART_COLORS.active}
                  isAnimationActive={false}
                  radius={[0, 4, 4, 0]}
                />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">
            Per-Project Breakdown
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Project</TableHead>
                <TableHead className="text-right">Runs</TableHead>
                <TableHead className="text-right">Compute</TableHead>
                <TableHead className="text-right">AI Cost</TableHead>
                <TableHead className="text-right">Total</TableHead>
                <TableHead className="text-right">Budget</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {costs.map((entry) => {
                const budget = (entry as ProjectCostWithBudget)
                  .monthly_budget_microusd;
                const budgetAction = (entry as ProjectCostWithBudget)
                  .budget_action;
                const hasBudget = budget !== undefined && budget > 0;
                const budgetPct = hasBudget
                  ? Math.min((entry.total_microusd / budget) * 100, 100)
                  : 0;

                return (
                  <TableRow key={entry.project_id}>
                    <TableCell className="font-medium">{entry.name}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {entry.runs.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(entry.compute_microusd)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(entry.ai_microusd)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(entry.total_microusd)}
                    </TableCell>
                    <TableCell className="text-right">
                      {hasBudget ? (
                        <div className="flex items-center justify-end gap-2">
                          <div className="w-16">
                            <Progress className="h-1.5" value={budgetPct} />
                          </div>
                          <span className="text-muted-foreground text-xs tabular-nums">
                            {formatMicroUsd(budget)}
                          </span>
                          <Button
                            className="h-6 px-1.5 text-xs"
                            onClick={() =>
                              setBudgetDialog({
                                projectId: entry.project_id,
                                projectName: entry.name,
                                budgetMicro: budget,
                                action: budgetAction || "notify",
                              })
                            }
                            size="sm"
                            variant="ghost"
                          >
                            Edit
                          </Button>
                        </div>
                      ) : (
                        <Button
                          className="h-6 px-1.5 text-xs"
                          onClick={() =>
                            setBudgetDialog({
                              projectId: entry.project_id,
                              projectName: entry.name,
                              budgetMicro: -1,
                              action: "notify",
                            })
                          }
                          size="sm"
                          variant="ghost"
                        >
                          Set Budget
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {budgetDialog && (
        <ProjectBudgetDialog
          currentAction={budgetDialog.action}
          currentBudgetMicro={budgetDialog.budgetMicro}
          onOpenChange={(open) => !open && setBudgetDialog(null)}
          open={!!budgetDialog}
          projectId={budgetDialog.projectId}
          projectName={budgetDialog.projectName}
        />
      )}
    </div>
  );
};

type ProjectCostWithBudget = {
  project_id: string;
  name: string;
  runs: number;
  compute_microusd: number;
  ai_microusd: number;
  total_microusd: number;
  monthly_budget_microusd?: number;
  budget_action?: string;
};

export default ProjectCostsTab;
