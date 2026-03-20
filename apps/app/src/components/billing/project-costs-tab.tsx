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
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { formatMicroUsd } from "@/lib/format";

export function ProjectCostsTab() {
  const { data: costs } = useQuery(projectCostsQueryOptions());

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
        <Card>
          <CardContent className="p-4">
            <p className="text-muted-foreground text-xs">Total Runs</p>
            <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
              {totals.runs.toLocaleString()}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-muted-foreground text-xs">Compute Cost</p>
            <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
              {formatMicroUsd(totals.compute)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-muted-foreground text-xs">AI Cost</p>
            <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
              {formatMicroUsd(totals.ai)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-muted-foreground text-xs">Total Cost</p>
            <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
              {formatMicroUsd(totals.total)}
            </p>
          </CardContent>
        </Card>
      </div>

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
              </TableRow>
            </TableHeader>
            <TableBody>
              {costs.map((entry) => (
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
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
