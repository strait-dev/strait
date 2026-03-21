import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Progress } from "@strait/ui/components/progress";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { projectBudgetQueryOptions } from "@/hooks/billing/use-project-budget";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { formatMicroUsd } from "@/lib/format";

const ProjectCostCard = ({
  activeProjectId,
}: {
  activeProjectId: string;
}) => {
  const { data: costs } = useQuery(projectCostsQueryOptions());
  const { data: budget } = useQuery(projectBudgetQueryOptions(activeProjectId));
  const project = costs?.find((c) => c.project_id === activeProjectId);

  if (!project) {
    return null;
  }

  const budgetMicro = budget?.monthly_budget_microusd;
  const hasBudget = budgetMicro !== undefined && budgetMicro > 0;
  const budgetPct = hasBudget
    ? Math.min((project.total_microusd / budgetMicro) * 100, 100)
    : 0;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">
          This Project's Cost
        </CardTitle>
        <Button render={<Link to="/app/billing" />} size="sm" variant="link">
          View Billing
        </Button>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-3 gap-4">
          <div>
            <p className="text-muted-foreground text-xs">Compute</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.compute_microusd)}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground text-xs">AI Cost</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.ai_microusd)}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground text-xs">Total</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.total_microusd)}
            </p>
          </div>
        </div>
        {hasBudget && (
          <div className="mt-3">
            <div className="mb-1 flex items-center justify-between">
              <p className="text-muted-foreground text-xs">
                Budget: {formatMicroUsd(budgetMicro ?? 0)}
              </p>
              <p className="text-muted-foreground text-xs tabular-nums">
                {budgetPct.toFixed(0)}%
              </p>
            </div>
            <Progress className="h-1.5" value={budgetPct} />
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default ProjectCostCard;
