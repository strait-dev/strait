import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { BarChart } from "@strait/ui/components/charts";
import { MetricCard } from "@strait/ui/components/metric-card";
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
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { formatMicroUsd } from "@/lib/format";
import ProjectBudgetDialog from "./project-budget-dialog";

type BudgetDialogState = {
  projectId: string;
  projectName: string;
  budgetMicro: number;
  action: string;
} | null;

const COST_BY_PROJECT_CONFIG = {
  total_microusd: { label: "Total cost", color: "chart-3" },
} satisfies ChartConfig;

const ProjectCostsTab = () => {
  const { data: costs } = useQuery(projectCostsQueryOptions());
  const [budgetDialog, setBudgetDialog] = useState<BudgetDialogState>(null);

  const isEmpty = !costs || costs.length === 0;

  const totals = (costs ?? []).reduce(
    (acc, c) => ({
      runs: acc.runs + c.runs,
      spend: acc.spend + c.spend_microusd,
      total: acc.total + c.total_microusd,
    }),
    { runs: 0, spend: 0, total: 0 }
  );

  const sortedCosts = [...(costs ?? [])].sort(
    (a, b) => b.total_microusd - a.total_microusd
  );

  if (isEmpty) {
    return (
      <Card>
        <CardContent className="flex h-48 items-center justify-center">
          <ChartEmptyState message="No project cost data for this billing period." />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
        <MetricCard
          size="sm"
          title="Total runs"
          value={totals.runs.toLocaleString()}
        />
        <MetricCard
          size="sm"
          title="Run spend"
          value={formatMicroUsd(totals.spend)}
        />
        <MetricCard
          size="sm"
          title="Total cost"
          value={formatMicroUsd(totals.total)}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">Cost by project</CardTitle>
        </CardHeader>
        <CardContent>
          <BarChart
            config={COST_BY_PROJECT_CONFIG}
            containerHeight={Math.max(200, costs.length * 36)}
            data={sortedCosts}
            dataKey="name"
            layout="vertical"
            legend={false}
            valueFormatter={formatMicroUsd}
            yAxisProps={{ width: 100 }}
          />
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
                <TableHead className="text-right">Run spend</TableHead>
                <TableHead className="text-right">Total</TableHead>
                <TableHead className="text-right">Budget</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {costs.map((entry) => {
                const budget = entry.monthly_budget_microusd;
                const budgetAction = entry.budget_action;
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
                      {formatMicroUsd(entry.spend_microusd)}
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
                            onClick={() =>
                              setBudgetDialog({
                                projectId: entry.project_id,
                                projectName: entry.name,
                                budgetMicro: budget,
                                action: budgetAction || "notify",
                              })
                            }
                            size="xs"
                            variant="ghost"
                          >
                            Edit
                          </Button>
                        </div>
                      ) : (
                        <Button
                          onClick={() =>
                            setBudgetDialog({
                              projectId: entry.project_id,
                              projectName: entry.name,
                              budgetMicro: -1,
                              action: "notify",
                            })
                          }
                          size="xs"
                          variant="ghost"
                        >
                          Set budget
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

export default ProjectCostsTab;
