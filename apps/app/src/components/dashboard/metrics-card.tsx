import { HugeiconsIcon } from "@hugeicons/react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { cn } from "@strait/ui/utils/index";
import { Bar, BarChart, ResponsiveContainer, Tooltip } from "recharts";
import { ArrowDownRightIcon, ArrowUpRightIcon } from "@/lib/icons";
import ChartTooltip from "./chart-tooltip";

type MetricsCardProps = {
  title: string;
  value: string | number;
  change?: { value: number; label: string };
  icon?: typeof ArrowUpRightIcon;
  description?: string;
  className?: string;
  chartData?: number[];
  chartColor?: string;
};

const MetricsCard = ({
  title,
  value,
  change,
  icon,
  description,
  className,
  chartData,
  chartColor = "var(--color-chart-1)",
}: MetricsCardProps) => {
  const isPositive = change && change.value >= 0;
  const barData = chartData?.map((v) => ({ value: v }));

  const labelMap = {
    value: { label: title, color: chartColor },
  };

  return (
    <Card className={cn("overflow-visible", className)}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="font-medium text-muted-foreground text-sm">
          {title}
        </CardTitle>
        {icon && (
          <HugeiconsIcon className="size-4 text-muted-foreground" icon={icon} />
        )}
      </CardHeader>
      <CardContent>
        <div className="font-normal text-2xl tabular-nums">{value}</div>
        {change && (
          <div className="mt-1 flex items-center gap-1 text-xs">
            <HugeiconsIcon
              className={cn(
                "size-3.5",
                isPositive ? "text-success" : "text-destructive"
              )}
              icon={isPositive ? ArrowUpRightIcon : ArrowDownRightIcon}
            />
            <span className={isPositive ? "text-success" : "text-destructive"}>
              {isPositive ? "+" : ""}
              {change.value}%
            </span>
            <span className="text-muted-foreground">{change.label}</span>
          </div>
        )}
        {description && (
          <p className="mt-1 text-muted-foreground text-xs">{description}</p>
        )}
        {barData && barData.length > 0 && (
          <div className="relative z-10 mt-3 h-[32px]">
            <ResponsiveContainer
              height="100%"
              minHeight={1}
              minWidth={1}
              width="100%"
            >
              <BarChart data={barData}>
                <Tooltip
                  content={<ChartTooltip labelMap={labelMap} />}
                  cursor={{ fill: "var(--muted)" }}
                  wrapperStyle={{ zIndex: 50 }}
                />
                <Bar dataKey="value" fill={chartColor} radius={[2, 2, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default MetricsCard;
