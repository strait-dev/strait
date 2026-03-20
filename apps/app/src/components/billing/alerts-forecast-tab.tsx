import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { anomalyAlertsQueryOptions } from "@/hooks/billing/use-anomaly-alerts";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";

const SEVERITY_VARIANT: Record<
  string,
  "default" | "secondary" | "destructive"
> = {
  warning: "secondary",
  high: "default",
  critical: "destructive",
};

export function AlertsForecastTab() {
  const { data: alerts } = useQuery(anomalyAlertsQueryOptions());
  const { data: forecast } = useQuery(usageForecastQueryOptions());
  const navigate = useNavigate();

  return (
    <div className="space-y-6">
      {/* Anomaly Alerts */}
      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">Anomaly Alerts</CardTitle>
        </CardHeader>
        <CardContent>
          {!alerts || alerts.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No anomalies detected. Spending patterns look normal.
            </p>
          ) : (
            <div className="space-y-3">
              {alerts.map((alert) => (
                <Card key={`${alert.severity}-${alert.top_contributor}`}>
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <Badge
                            variant={
                              SEVERITY_VARIANT[alert.severity] ?? "secondary"
                            }
                          >
                            {alert.severity}
                          </Badge>
                          <span className="text-muted-foreground text-xs">
                            {alert.spike_ratio.toFixed(1)}x spike
                          </span>
                        </div>
                        <p className="text-sm">
                          Today: ${alert.today_spend.toFixed(2)} vs 7d avg: $
                          {alert.avg_7d_spend.toFixed(2)}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          Top contributor: {alert.top_contributor}
                        </p>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Forecast */}
      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">Usage Forecast</CardTitle>
        </CardHeader>
        <CardContent>
          {forecast ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
                <Card>
                  <CardContent className="p-4">
                    <p className="text-muted-foreground text-xs">
                      Projected Runs
                    </p>
                    <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
                      {forecast.projected_monthly_runs.toLocaleString()}
                    </p>
                  </CardContent>
                </Card>
                <Card>
                  <CardContent className="p-4">
                    <p className="text-muted-foreground text-xs">
                      Projected Compute
                    </p>
                    <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
                      ${forecast.projected_monthly_compute_usd.toFixed(2)}
                    </p>
                  </CardContent>
                </Card>
                <Card>
                  <CardContent className="p-4">
                    <p className="text-muted-foreground text-xs">
                      Projected AI Cost
                    </p>
                    <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
                      ${forecast.projected_monthly_ai_cost_usd.toFixed(2)}
                    </p>
                  </CardContent>
                </Card>
                <Card>
                  <CardContent className="p-4">
                    <p className="text-muted-foreground text-xs">
                      Days Until Limit
                    </p>
                    <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
                      {forecast.days_until_limit === -1
                        ? "N/A"
                        : forecast.days_until_limit}
                    </p>
                  </CardContent>
                </Card>
              </div>

              {forecast.recommended_plan && (
                <Card className="border-blue-200 dark:border-blue-800">
                  <CardContent className="flex items-center justify-between p-4">
                    <p className="text-sm">
                      Based on your projected usage, we recommend the{" "}
                      <span className="font-medium">
                        {forecast.recommended_plan.charAt(0).toUpperCase() +
                          forecast.recommended_plan.slice(1)}
                      </span>{" "}
                      plan.
                    </p>
                    <Button
                      onClick={() => navigate({ to: "/app/upgrade" })}
                      size="sm"
                      variant="outline"
                    >
                      View Plans
                    </Button>
                  </CardContent>
                </Card>
              )}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              Forecast data unavailable.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
