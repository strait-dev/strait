import { HugeiconsIcon } from "@hugeicons/react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { cn } from "@strait/ui/utils/index";
import { ArrowDownRightIcon, ArrowUpRightIcon } from "@/lib/icons";

type MetricsCardProps = {
  title: string;
  value: string | number;
  change?: { value: number; label: string };
  icon?: any;
  description?: string;
  className?: string;
};

export function MetricsCard({
  title,
  value,
  change,
  icon,
  description,
  className,
}: MetricsCardProps) {
  const isPositive = change && change.value >= 0;

  return (
    <Card className={cn(className)}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="font-medium text-muted-foreground text-sm">
          {title}
        </CardTitle>
        {icon && (
          <HugeiconsIcon
            className="text-muted-foreground"
            icon={icon}
            size={16}
          />
        )}
      </CardHeader>
      <CardContent>
        <div className="font-normal text-2xl">{value}</div>
        {change && (
          <div className="mt-1 flex items-center gap-1 text-xs">
            <HugeiconsIcon
              className={isPositive ? "text-chart-1" : "text-chart-4"}
              icon={isPositive ? ArrowUpRightIcon : ArrowDownRightIcon}
              size={14}
            />
            <span className={isPositive ? "text-chart-1" : "text-chart-4"}>
              {isPositive ? "+" : ""}
              {change.value}%
            </span>
            <span className="text-muted-foreground">{change.label}</span>
          </div>
        )}
        {description && (
          <p className="mt-1 text-muted-foreground text-xs">{description}</p>
        )}
      </CardContent>
    </Card>
  );
}
