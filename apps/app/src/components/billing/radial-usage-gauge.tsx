import { HugeiconsIcon } from "@hugeicons/react";
import { Card, CardContent } from "@strait/ui/components/card";
import {
  PolarAngleAxis,
  RadialBar,
  RadialBarChart,
  ResponsiveContainer,
} from "recharts";
import { CheckCircleIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

type RadialUsageGaugeProps = {
  label: string;
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

function getGaugeColor(percent: number): string {
  if (percent >= 90) {
    return CHART_COLORS.error;
  }
  if (percent >= 70) {
    return CHART_COLORS.warning;
  }
  return CHART_COLORS.active;
}

export function RadialUsageGauge({
  label,
  used,
  limit,
  percent,
  display,
}: RadialUsageGaugeProps) {
  const isUnlimited = limit === -1;
  const displayValue = display || `${used.toLocaleString()}`;
  const limitDisplay = isUnlimited ? "Unlimited" : limit.toLocaleString();
  const color = getGaugeColor(percent);

  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        {isUnlimited ? (
          <div className="flex h-[120px] items-center justify-center">
            <HugeiconsIcon
              className="text-success"
              icon={CheckCircleIcon}
              size={32}
            />
          </div>
        ) : (
          <div className="relative h-[120px]">
            <ResponsiveContainer height="100%" width="100%">
              <RadialBarChart
                barSize={8}
                cx="50%"
                cy="50%"
                data={[{ value: Math.min(percent, 100) }]}
                endAngle={-270}
                innerRadius="65%"
                outerRadius="85%"
                startAngle={90}
              >
                <PolarAngleAxis
                  angleAxisId={0}
                  domain={[0, 100]}
                  tick={false}
                  type="number"
                />
                <RadialBar
                  background={{ fill: "var(--muted)" }}
                  cornerRadius={4}
                  dataKey="value"
                  fill={color}
                  isAnimationActive={false}
                />
              </RadialBarChart>
            </ResponsiveContainer>
            <div className="absolute inset-0 flex flex-col items-center justify-center">
              <span className="font-medium text-foreground text-sm tabular-nums">
                {displayValue}
              </span>
              <span className="text-muted-foreground text-xs tabular-nums">
                / {limitDisplay}
              </span>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
