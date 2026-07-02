import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { BarChart, LineChart } from "@strait/ui/components/charts";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { toast } from "@strait/ui/components/toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchUsageExportCsv,
  fetchUsageExportPdf,
} from "@/hooks/billing/use-usage-export";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { queryKeys } from "@/hooks/query-keys";
import { getPostHog } from "@/lib/analytics";
import { formatMicroUsd } from "@/lib/format";
import { ActivityIcon } from "@/lib/icons";

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function currentPeriod() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
}

const COST_CHART_CONFIG: ChartConfig = {
  spend_microusd: {
    label: "Run spend",
    color: "chart-3",
  },
};

const RUNS_CHART_CONFIG: ChartConfig = {
  runs_count: {
    label: "Runs",
    color: "chart-5",
  },
};

const UsageHistoryTab = () => {
  const queryClient = useQueryClient();
  const { data: history } = useQuery(usageHistoryQueryOptions());

  const isEmpty = !history || history.length === 0;

  const csvExport = useMutation({
    mutationFn: async () => {
      const period = currentPeriod();
      const csv = await fetchUsageExportCsv(period);
      triggerDownload(
        new Blob([csv], { type: "text/csv" }),
        `usage-${period}.csv`
      );
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.billing.usageHistory.queryKey,
      });
      toast.success("CSV exported successfully");
      getPostHog()?.capture("usage_export_csv");
    },
    onError: () => toast.error("Failed to export CSV"),
  });

  const pdfExport = useMutation({
    mutationFn: async () => {
      const period = currentPeriod();
      const base64 = await fetchUsageExportPdf(period);
      if (!base64) {
        return;
      }
      const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
      triggerDownload(
        new Blob([bytes], { type: "application/pdf" }),
        `usage-${period}.pdf`
      );
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.billing.usageHistory.queryKey,
      });
      toast.success("PDF exported successfully");
      getPostHog()?.capture("usage_export_pdf");
    },
    onError: () => toast.error("Failed to export PDF"),
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="font-medium text-sm">Daily usage</CardTitle>
          <div className="flex items-center gap-2">
            <Button
              disabled={isEmpty || csvExport.isPending}
              onClick={() => csvExport.mutate()}
              variant="outline"
            >
              {csvExport.isPending ? "Exporting..." : "Download CSV"}
            </Button>
            <Button
              disabled={isEmpty || pdfExport.isPending}
              onClick={() => pdfExport.mutate()}
              variant="outline"
            >
              {pdfExport.isPending ? "Exporting..." : "Download PDF"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {isEmpty ? (
            <div className="h-[280px]">
              <ChartEmptyState
                icon={ActivityIcon}
                message="No usage data yet for this billing period."
              />
            </div>
          ) : (
            <div className="grid gap-6 lg:grid-cols-2">
              <BarChart
                config={COST_CHART_CONFIG}
                containerHeight={260}
                data={history}
                dataKey="date"
                type="stacked"
                valueFormatter={formatMicroUsd}
                xAxisProps={{
                  tickFormatter: (value: string) => value.slice(5),
                }}
              />
              <LineChart
                config={RUNS_CHART_CONFIG}
                containerHeight={260}
                data={history}
                dataKey="date"
                valueFormatter={(value) => value.toLocaleString()}
                xAxisProps={{
                  tickFormatter: (value: string) => value.slice(5),
                }}
              />
            </div>
          )}
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
                  <TableHead className="text-right">Run spend</TableHead>
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
                      {formatMicroUsd(entry.spend_microusd)}
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
};

export default UsageHistoryTab;
