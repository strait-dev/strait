import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { spendingLimitQueryOptions } from "@/hooks/billing/use-spending-limit";

export function SpendingLimitsTab() {
  const { data: spending } = useQuery(spendingLimitQueryOptions());

  if (!spending) {
    return (
      <Card>
        <CardContent className="flex h-48 items-center justify-center">
          <p className="text-muted-foreground text-sm">
            Spending limit data unavailable.
          </p>
        </CardContent>
      </Card>
    );
  }

  const planName =
    spending.plan_tier.charAt(0).toUpperCase() + spending.plan_tier.slice(1);
  const percent =
    spending.spending_limit_usd > 0
      ? (spending.current_spend_usd / spending.spending_limit_usd) * 100
      : 0;

  return (
    <div className="space-y-6">
      {spending.is_hard_capped && (
        <Card className="border-yellow-200 dark:border-yellow-800">
          <CardContent className="p-4">
            <p className="text-sm text-yellow-800 dark:text-yellow-200">
              Hard spending cap is enabled. Services will be paused when the
              limit is reached.
            </p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 font-medium text-sm">
            Spending Limit
            <Badge variant="default">{planName}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div>
            <div className="flex items-baseline justify-between">
              <p className="text-muted-foreground text-sm">Current Spend</p>
              <p className="font-medium text-foreground text-lg tabular-nums">
                ${spending.current_spend_usd.toFixed(2)}
                <span className="text-muted-foreground text-sm">
                  {" "}
                  / ${spending.spending_limit_usd.toFixed(2)}
                </span>
              </p>
            </div>
            <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-foreground transition-all"
                style={{ width: `${Math.min(percent, 100)}%` }}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-muted-foreground text-xs">Included Credit</p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                ${spending.included_credit_usd.toFixed(2)}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Overage Spend</p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                ${spending.overage_spend_usd.toFixed(2)}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
