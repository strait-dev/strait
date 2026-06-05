import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Label } from "@strait/ui/components/label";
import { MetricCard } from "@strait/ui/components/metric-card";
import {
  NoticeBanner,
  NoticeBannerAction,
} from "@strait/ui/components/notice-banner";
import { NumberInputWithChevrons } from "@strait/ui/components/number-input-with-chevrons";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { anomalyAlertsQueryOptions } from "@/hooks/billing/use-anomaly-alerts";
import {
  anomalyConfigQueryOptions,
  useSetAnomalyConfig,
} from "@/hooks/billing/use-anomaly-config";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";
import { capitalize, formatMicroUsd } from "@/lib/format";

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
                          Today: {formatMicroUsd(alert.today_spend ?? 0)} vs 7d
                          avg: {formatMicroUsd(alert.avg_7d_spend ?? 0)}
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
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
                <MetricCard
                  size="sm"
                  title="Projected Runs"
                  value={(
                    forecast.projected_monthly_runs ?? 0
                  ).toLocaleString()}
                />
                <MetricCard
                  size="sm"
                  title="Projected Spend"
                  value={`$${(forecast.projected_monthly_spend_usd ?? 0).toFixed(2)}`}
                />
                <MetricCard
                  size="sm"
                  title="Days Until Limit"
                  value={
                    forecast.days_until_limit === -1
                      ? "N/A"
                      : String(forecast.days_until_limit)
                  }
                />
              </div>

              {forecast.recommended_plan && (
                <NoticeBanner
                  action={
                    <NoticeBannerAction>
                      <Button
                        onClick={() => navigate({ to: "/app/upgrade" })}
                        variant="info-outline"
                      >
                        View Plans
                      </Button>
                    </NoticeBannerAction>
                  }
                  title="Plan recommendation"
                  variant="info"
                >
                  Based on your projected usage, we recommend the{" "}
                  {capitalize(forecast.recommended_plan)} plan.
                </NoticeBanner>
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
  const [warning, setWarning] = useState(warningThreshold);
  const [critical, setCritical] = useState(criticalThreshold);
  const mutation = useSetAnomalyConfig();

  const handleSave = () => {
    if (warning <= 1 || critical <= warning) {
      return;
    }
    mutation.mutate({ warningThreshold: warning, criticalThreshold: critical });
  };

  const isDirty =
    warning !== warningThreshold || critical !== criticalThreshold;

  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="warning-threshold">Warning Threshold (x)</Label>
          <NumberInputWithChevrons
            id="warning-threshold"
            min={1.1}
            name="warning-threshold"
            onChange={setWarning}
            step={0.5}
            value={warning}
          />
          <p className="text-muted-foreground text-xs">
            Triggers a warning-level alert (default: 3x)
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="critical-threshold">Critical Threshold (x)</Label>
          <NumberInputWithChevrons
            id="critical-threshold"
            min={2}
            name="critical-threshold"
            onChange={setCritical}
            step={1}
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
            {mutation.isPending ? "Saving..." : "Save thresholds"}
          </Button>
        </div>
      )}
      {mutation.isSuccess && (
        <NoticeBanner
          className="mt-4"
          title="Thresholds updated"
          variant="success"
        >
          Anomaly detection thresholds are saved.
        </NoticeBanner>
      )}
    </>
  );
};

export default AlertsForecastTab;
