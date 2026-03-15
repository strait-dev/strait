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
import { Link } from "@tanstack/react-router";
import type { DisplayStatus } from "@/hooks/api/types";
import { StatusBadge } from "./status-badge";

type RecentRun = {
  id: string;
  job_name: string;
  status: DisplayStatus;
  duration: string;
  started_at: string;
};

const MOCK_RUNS: RecentRun[] = [
  {
    id: "run_a1b2c3",
    job_name: "payment-sync",
    status: "completed",
    duration: "1.2s",
    started_at: "2 min ago",
  },
  {
    id: "run_d4e5f6",
    job_name: "email-dispatch",
    status: "executing",
    duration: "0.8s",
    started_at: "5 min ago",
  },
  {
    id: "run_g7h8i9",
    job_name: "report-gen",
    status: "failed",
    duration: "4.5s",
    started_at: "12 min ago",
  },
  {
    id: "run_j1k2l3",
    job_name: "user-import",
    status: "queued",
    duration: "-",
    started_at: "15 min ago",
  },
  {
    id: "run_m4n5o6",
    job_name: "webhook-relay",
    status: "completed",
    duration: "0.3s",
    started_at: "22 min ago",
  },
  {
    id: "run_p7q8r9",
    job_name: "cache-warm",
    status: "timed_out",
    duration: "30.0s",
    started_at: "35 min ago",
  },
];

export function RecentRunsTable() {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Recent Runs</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-6">Run ID</TableHead>
              <TableHead>Job</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Duration</TableHead>
              <TableHead>Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {MOCK_RUNS.map((run) => (
              <TableRow key={run.id}>
                <TableCell className="pl-6">
                  <Link
                    className="font-mono text-sm hover:underline"
                    params={{ id: run.id }}
                    to="/app/runs/$id"
                  >
                    {run.id}
                  </Link>
                </TableCell>
                <TableCell className="text-sm">{run.job_name}</TableCell>
                <TableCell>
                  <StatusBadge status={run.status} />
                </TableCell>
                <TableCell className="font-mono text-sm tabular-nums">
                  {run.duration}
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {run.started_at}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
