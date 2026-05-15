import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import { Label } from "@strait/ui/components/label";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { anomalyAlertsQueryOptions } from "@/hooks/billing/use-anomaly-alerts";
import {
  anomalyConfigQueryOptions,
  useSetAnomalyConfig,
} from "@/hooks/billing/use-anomaly-config";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";
import { capitalize } from "@/lib/format";
import UsageStatCard from "./usage-stat-card";

const SEVERITY_VARIANT: Record<
  string,
  "default" | "secondary" | "destructive"
> = {
  warning: "secondary",
  high: "default",
  critical: "destructive",
};

const AlertsForecastTab = () => {
  const { data: alerts } = useQuery(anomalyAlertsQueryOptions());
  const { data: forecast } = useQuery(usageForecastQueryOptions());
  const { data: config } = useQuery(anomalyConfigQueryOptions());
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
                <Card
                  key={`${alert.severity}-${alert.top_contributor}-${alert.spike_ratio}-${alert.today_spend}`}
                >
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
                            {(alert.spike_ratio ?? 0).toFixed(1)}x spike
                          </span>
                        </div>
                        <p className="text-sm">
                          Today: ${(alert.today_spend ?? 0).toFixed(2)} vs 7d
                          avg: ${(alert.avg_7d_spend ?? 0).toFixed(2)}
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

          <p className="mt-3 text-muted-foreground text-xs">
            Anomaly alerts are automatically sent to your configured
            notification channels (webhooks, Slack, Discord).
          </p>
        </CardContent>
      </Card>

      {/* Configure Thresholds */}
      <ThresholdConfigCard
        criticalThreshold={config?.critical_threshold ?? 10.0}
        warningThreshold={config?.warning_threshold ?? 3.0}
      />

      {/* Forecast */}
      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">Usage Forecast</CardTitle>
        </CardHeader>
        <CardContent>
          {forecast ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
                <UsageStatCard
                  label="Projected Runs"
                  value={(
                    forecast.projected_monthly_runs ?? 0
                  ).toLocaleString()}
                />
                <UsageStatCard
                  label="Projected Compute"
                  value={`$${(forecast.projected_monthly_compute_usd ?? 0).toFixed(2)}`}
                />
                <UsageStatCard
                  label="Projected AI Cost"
                  value={`$${(forecast.projected_monthly_ai_cost_usd ?? 0).toFixed(2)}`}
                />
                <UsageStatCard
                  label="Days Until Limit"
                  value={
                    forecast.days_until_limit === -1
                      ? "N/A"
                      : String(forecast.days_until_limit)
                  }
                />
              </div>

              {forecast.recommended_plan && (
                <Card className="border-info/30">
                  <CardContent className="flex items-center justify-between p-4">
                    <p className="text-sm">
                      Based on your projected usage, we recommend the{" "}
                      <span className="font-medium">
                        {capitalize(forecast.recommended_plan)}
                      </span>{" "}
                      plan.
                    </p>
                    <Button
                      onClick={() => navigate({ to: "/app/upgrade" })}
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
};

const ThresholdConfigCard = ({
  warningThreshold,
  criticalThreshold,
}: {
  warningThreshold: number;
  criticalThreshold: number;
}) => (
  <Card>
    <CardHeader>
      <CardTitle className="font-medium text-sm">
        Anomaly Detection Thresholds
      </CardTitle>
    </CardHeader>
    <CardContent>
      <p className="mb-4 text-muted-foreground text-sm">
        Configure the spike ratio thresholds that trigger anomaly alerts. A
        spike ratio compares today&apos;s spend against the 7-day average.
      </p>
      <ThresholdForm
        criticalThreshold={criticalThreshold}
        key={`${warningThreshold}-${criticalThreshold}`}
        warningThreshold={warningThreshold}
      />
    </CardContent>
  </Card>
);

const ThresholdForm = ({
  warningThreshold,
  criticalThreshold,
}: {
  warningThreshold: number;
  criticalThreshold: number;
}) => {
  const [warning, setWarning] = useState(String(warningThreshold));
  const [critical, setCritical] = useState(String(criticalThreshold));
  const mutation = useSetAnomalyConfig();

  const handleSave = () => {
    const w = Number.parseFloat(warning);
    const c = Number.parseFloat(critical);
    if (Number.isNaN(w) || Number.isNaN(c) || w <= 1 || c <= w) {
      return;
    }
    mutation.mutate({ warningThreshold: w, criticalThreshold: c });
  };

  const isDirty =
    warning !== String(warningThreshold) ||
    critical !== String(criticalThreshold);

  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="warning-threshold">Warning Threshold (x)</Label>
          <Input
            id="warning-threshold"
            min="1.1"
            onChange={(e) => setWarning(e.target.value)}
            step="0.5"
            type="number"
            value={warning}
          />
          <p className="text-muted-foreground text-xs">
            Triggers a warning-level alert (default: 3x)
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="critical-threshold">Critical Threshold (x)</Label>
          <Input
            id="critical-threshold"
            min="2"
            onChange={(e) => setCritical(e.target.value)}
            step="1"
            type="number"
            value={critical}
          />
          <p className="text-muted-foreground text-xs">
            Triggers a critical-level alert (default: 10x)
          </p>
        </div>
      </div>
      {isDirty && (
        <div className="mt-4 flex justify-end">
          <Button disabled={mutation.isPending} onClick={handleSave}>
            {mutation.isPending ? "Saving..." : "Save Thresholds"}
          </Button>
        </div>
      )}
      {mutation.isSuccess && (
        <p className="mt-2 text-right text-success text-xs">
          Thresholds updated successfully.
        </p>
      )}
    </>
  );
};

export default AlertsForecastTab;
