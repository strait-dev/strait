import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";

function UsageGauge({
  label,
  used,
  limit,
  percent,
  display,
}: {
  label: string;
  used: number;
  limit: number;
  percent: number;
  display?: string;
}) {
  const isUnlimited = limit === -1;
  const displayValue = display || `${used.toLocaleString()}`;
  const limitDisplay = isUnlimited ? "Unlimited" : limit.toLocaleString();

  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
          {displayValue}
        </p>
        <p className="text-muted-foreground text-xs">/ {limitDisplay}</p>
        {!isUnlimited && (
          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-foreground transition-all"
              style={{ width: `${Math.min(percent, 100)}%` }}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function UsageDashboard() {
  const { data: usage, isLoading } = useQuery(orgUsageQueryOptions());
  const navigate = useNavigate();

  const handleManageBilling = async () => {
    const result = await getCustomerPortalUrlServerFn();
    if (result.url) {
      window.location.href = result.url;
    }
  };

  if (isLoading || !usage) {
    return (
      <div className="flex h-48 items-center justify-center">
        <p className="text-muted-foreground text-sm">Loading usage data...</p>
      </div>
    );
  }

  const planName = usage.plan.charAt(0).toUpperCase() + usage.plan.slice(1);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-normal text-foreground text-lg tracking-tight">
            Usage & Billing
          </h2>
          <p className="text-muted-foreground text-sm">
            Current plan: <Badge variant="default">{planName}</Badge>
            {usage.period.start && (
              <span className="ml-2">
                Billing period: {usage.period.start} - {usage.period.end}
              </span>
            )}
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            onClick={() => navigate({ to: "/app/upgrade" })}
            size="sm"
            variant="outline"
          >
            Upgrade Plan
          </Button>
          <Button onClick={handleManageBilling} size="sm" variant="outline">
            Manage Billing
          </Button>
        </div>
      </div>

      {/* Quota Gauges */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <UsageGauge
          display={usage.usage.runs_today.display}
          label="Runs Today"
          limit={usage.usage.runs_today.limit}
          percent={usage.usage.runs_today.percent}
          used={usage.usage.runs_today.used}
        />
        <UsageGauge
          label="Concurrent Runs"
          limit={usage.usage.concurrent_runs.limit}
          percent={usage.usage.concurrent_runs.percent}
          used={usage.usage.concurrent_runs.used}
        />
        <UsageGauge
          display={usage.usage.compute_credit.display}
          label="Compute Credit"
          limit={usage.usage.compute_credit.limit}
          percent={usage.usage.compute_credit.percent}
          used={usage.usage.compute_credit.used}
        />
        <UsageGauge
          label="AI Assistant"
          limit={usage.usage.ai_assistant_messages_today.limit}
          percent={usage.usage.ai_assistant_messages_today.percent}
          used={usage.usage.ai_assistant_messages_today.used}
        />
      </div>

      {/* Resources */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Resources</CardTitle>
          <CardDescription>Organization resource allocation</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <p className="text-muted-foreground text-xs">Projects</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.projects.used} /{" "}
                {usage.usage.projects.limit === -1
                  ? "Unlimited"
                  : usage.usage.projects.limit}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Members</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.members.used} /{" "}
                {usage.usage.members.limit === -1
                  ? "Unlimited"
                  : usage.usage.members.limit}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Regions</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.regions_available}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Alerts */}
      {usage.alerts.length > 0 && (
        <Card className="border-yellow-200 dark:border-yellow-800">
          <CardHeader>
            <CardTitle className="text-sm">Alerts</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {usage.alerts.map((alert) => (
                <div
                  className="flex items-center justify-between rounded-custom bg-yellow-50 p-2 dark:bg-yellow-950"
                  key={alert.dimension}
                >
                  <span className="text-sm text-yellow-800 dark:text-yellow-200">
                    {alert.message}
                  </span>
                  <Button
                    onClick={() => navigate({ to: "/app/upgrade" })}
                    size="sm"
                    variant="outline"
                  >
                    Upgrade
                  </Button>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <p className="text-center text-muted-foreground text-xs">
        Retention: {usage.usage.retention_days} day
        {usage.usage.retention_days === 1 ? "" : "s"}
      </p>
    </div>
  );
}
