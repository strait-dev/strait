import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import { Label } from "@strait/ui/components/label";
import { Shell } from "@strait/ui/components/shell";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import {
  notifyEscalationQueryOptions,
  useAcknowledgeNotifyEscalation,
  useCompleteNotifyEscalation,
} from "@/hooks/api/use-notify";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/escalations")({
  loader: ({ context }) => {
    const { session } = context as AppRouteContext;
    return {
      hasProject: !!session.user.activeProjectId,
      session,
    };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyEscalationsPage,
});

function NotifyEscalationsPage() {
  const { hasProject, session } = Route.useLoaderData();

  const [stepRunID, setStepRunID] = useState("");
  const [lastActionResult, setLastActionResult] = useState("");

  const escalationQuery = useQuery({
    ...notifyEscalationQueryOptions(stepRunID),
    enabled: hasProject && !!stepRunID,
  });

  const acknowledge = useAcknowledgeNotifyEscalation();
  const complete = useCompleteNotifyEscalation();

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const runAction = async (action: "ack" | "complete" | "fail") => {
    if (!stepRunID.trim()) {
      toast.error("Step run ID is required");
      return;
    }

    if (action === "ack") {
      const result = await toast.promise(
        acknowledge.mutateAsync({ stepRunId: stepRunID.trim() }),
        {
          loading: "Acknowledging escalation...",
          success: "Escalation acknowledged",
          error: "Failed to acknowledge escalation",
        }
      );
      setLastActionResult(JSON.stringify(result, null, 2));
      await escalationQuery.refetch();
      return;
    }

    const result = await toast.promise(
      complete.mutateAsync({
        stepRunId: stepRunID.trim(),
        status: action === "complete" ? "completed" : "failed",
      }),
      {
        loading:
          action === "complete"
            ? "Completing escalation..."
            : "Failing escalation...",
        success:
          action === "complete"
            ? "Escalation completed"
            : "Escalation marked failed",
        error: "Failed to update escalation",
      }
    );

    setLastActionResult(JSON.stringify(result, null, 2));
    await escalationQuery.refetch();
  };

  const escalation = escalationQuery.data;

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Escalation lookup</CardTitle>
            <CardDescription>
              Load escalation state by step run ID, then acknowledge or
              complete.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="step-run-id">Step run ID</Label>
              <Input
                id="step-run-id"
                onChange={(event) => setStepRunID(event.target.value)}
                placeholder="step run id"
                value={stepRunID}
              />
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => escalationQuery.refetch()}
                variant="outline"
              >
                Refresh state
              </Button>
              <Button onClick={() => runAction("ack")} variant="secondary">
                Acknowledge
              </Button>
              <Button onClick={() => runAction("complete")}>Complete</Button>
              <Button onClick={() => runAction("fail")} variant="destructive">
                Mark failed
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Current state</CardTitle>
            <CardDescription>
              Live escalation state from the API lookup.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {escalation ? (
              <>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground text-sm">Status</span>
                  <NotifyStatusBadge status={escalation.status} />
                </div>
                <p className="text-sm">
                  <span className="text-muted-foreground">Step run:</span>{" "}
                  {escalation.step_run_id}
                </p>
                <p className="text-sm">
                  <span className="text-muted-foreground">Workflow run:</span>{" "}
                  {escalation.workflow_run_id}
                </p>
                <p className="text-sm">
                  <span className="text-muted-foreground">Tier:</span>{" "}
                  {escalation.current_tier} / {escalation.total_tiers}
                </p>
                <p className="text-sm">
                  <span className="text-muted-foreground">Acknowledged:</span>{" "}
                  {escalation.acknowledged ? "yes" : "no"}
                </p>
              </>
            ) : (
              <p className="text-muted-foreground text-sm">
                Enter a step run ID to load escalation state.
              </p>
            )}

            {escalationQuery.error ? (
              <p className="text-destructive text-sm">
                {(escalationQuery.error as Error).message}
              </p>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-sm">Action result</CardTitle>
          <CardDescription>
            Last action API response, useful for incident handoffs.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Textarea
            className="min-h-[180px] font-mono text-xs"
            readOnly
            value={lastActionResult}
          />
        </CardContent>
      </Card>
    </Shell>
  );
}
