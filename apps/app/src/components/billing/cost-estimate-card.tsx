import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { costEstimateQueryOptions } from "@/hooks/billing/use-cost-estimate";
import { formatMicroUsd } from "@/lib/format";

const PRESETS = [
  { value: "micro", label: "Micro" },
  { value: "small", label: "Small" },
  { value: "medium", label: "Medium" },
  { value: "large", label: "Large" },
  { value: "large-2x", label: "Large 2x" },
];

type CostEstimateCardProps = {
  timeoutSecs: number;
};

export function CostEstimateCard({ timeoutSecs }: CostEstimateCardProps) {
  const [preset, setPreset] = useState("micro");

  const { data: estimate } = useQuery(
    costEstimateQueryOptions(preset, timeoutSecs)
  );

  const bestAlternative = estimate?.alternatives?.find(
    (a) => a.savings_pct > 0
  );

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="font-medium text-sm">Cost per Run</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground text-sm">Preset</span>
          <Select
            onValueChange={(v) => {
              if (v) {
                setPreset(v);
              }
            }}
            value={preset}
          >
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PRESETS.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground text-sm">Timeout</span>
          <span className="font-mono text-xs">{timeoutSecs}s</span>
        </div>

        {estimate ? (
          <>
            <div className="flex items-center justify-between gap-3 border-border border-t pt-3">
              <span className="text-muted-foreground text-sm">
                Estimated cost
              </span>
              <span className="font-medium tabular-nums">
                {formatMicroUsd(estimate.estimated_cost_microusd)}
              </span>
            </div>

            {bestAlternative && bestAlternative.savings_pct > 0 ? (
              <p className="text-muted-foreground text-xs">
                Save {bestAlternative.savings_pct}% with{" "}
                <span className="font-medium text-foreground">
                  {bestAlternative.preset}
                </span>
              </p>
            ) : null}

            {estimate.credit_info.estimated_runs_remaining > 0 ? (
              <p className="text-muted-foreground text-xs">
                ~
                {estimate.credit_info.estimated_runs_remaining.toLocaleString()}{" "}
                runs remaining with current credit
              </p>
            ) : null}
          </>
        ) : (
          <div className="flex items-center justify-between gap-3 border-border border-t pt-3">
            <span className="text-muted-foreground text-sm">
              Estimated cost
            </span>
            <span className="text-muted-foreground text-xs">-</span>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
