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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import {
  notifyPoliciesQueryOptions,
  useCreateNotifyPolicyOverride,
  useDeleteNotifyPolicyOverride,
  useUpdateNotifyPolicyOverride,
} from "@/hooks/api/use-notify";
import type { NotifyPolicyOverride } from "@/hooks/api/types";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/policies")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(notifyPoliciesQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyPoliciesPage,
});

function NotifyPoliciesPage() {
  const { hasProject, session } = Route.useLoaderData();

  const policiesQuery = useQuery({
    ...notifyPoliciesQueryOptions(),
    enabled: hasProject,
  });

  const createPolicy = useCreateNotifyPolicyOverride();
  const updatePolicy = useUpdateNotifyPolicyOverride();
  const deletePolicy = useDeleteNotifyPolicyOverride();

  const [selected, setSelected] = useState<NotifyPolicyOverride | null>(null);

  const [scopeType, setScopeType] = useState<"project" | "category" | "workflow_step">(
    "project"
  );
  const [scopeKey, setScopeKey] = useState("project");
  const [channel, setChannel] = useState("email");
  const [digestPolicy, setDigestPolicy] = useState("instant");
  const [retryAttempts, setRetryAttempts] = useState("");
  const [retryBaseDelay, setRetryBaseDelay] = useState("");
  const [retryMaxDelay, setRetryMaxDelay] = useState("");
  const [escalationTiers, setEscalationTiers] = useState("");
  const [escalationMinInterval, setEscalationMinInterval] = useState("");
  const [enabled, setEnabled] = useState(true);

  const policies = policiesQuery.data ?? [];

  const sortedPolicies = useMemo(
    () => [...policies].sort((a, b) => b.updated_at.localeCompare(a.updated_at)),
    [policies]
  );

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const toNumber = (value: string) => {
    if (!value.trim()) {
      return undefined;
    }
    const parsed = Number(value);
    if (Number.isNaN(parsed)) {
      return undefined;
    }
    return parsed;
  };

  const resetForm = () => {
    setSelected(null);
    setScopeType("project");
    setScopeKey("project");
    setChannel("email");
    setDigestPolicy("instant");
    setRetryAttempts("");
    setRetryBaseDelay("");
    setRetryMaxDelay("");
    setEscalationTiers("");
    setEscalationMinInterval("");
    setEnabled(true);
  };

  const upsert = async () => {
    if (!scopeKey.trim()) {
      toast.error("Scope key is required");
      return;
    }

    const payload = {
      digest_policy: digestPolicy as "instant" | "hourly" | "daily",
      retry_max_attempts: toNumber(retryAttempts),
      retry_base_delay_secs: toNumber(retryBaseDelay),
      retry_max_delay_secs: toNumber(retryMaxDelay),
      escalation_tiers: toNumber(escalationTiers),
      escalation_min_interval_secs: toNumber(escalationMinInterval),
      enabled,
    };

    if (selected) {
      await toast.promise(
        updatePolicy.mutateAsync({
          policyId: selected.id,
          ...payload,
        }),
        {
          loading: "Updating policy...",
          success: "Policy updated",
          error: "Failed to update policy",
        }
      );
    } else {
      await toast.promise(
        createPolicy.mutateAsync({
          scope_type: scopeType,
          scope_key: scopeKey.trim(),
          channel: channel.trim() || undefined,
          ...payload,
        }),
        {
          loading: "Creating policy...",
          success: "Policy created",
          error: "Failed to create policy",
        }
      );
    }

    resetForm();
  };

  const remove = async () => {
    if (!selected) {
      return;
    }

    await toast.promise(deletePolicy.mutateAsync({ policyId: selected.id }), {
      loading: "Deleting policy...",
      success: "Policy deleted",
      error: "Failed to delete policy",
    });

    resetForm();
  };

  const loadPolicyForEdit = (item: NotifyPolicyOverride) => {
    setSelected(item);
    setScopeType(item.scope_type);
    setScopeKey(item.scope_key);
    setChannel(item.channel || "");
    setDigestPolicy(item.digest_policy || "instant");
    setRetryAttempts(item.retry_max_attempts ? String(item.retry_max_attempts) : "");
    setRetryBaseDelay(
      item.retry_base_delay_secs ? String(item.retry_base_delay_secs) : ""
    );
    setRetryMaxDelay(item.retry_max_delay_secs ? String(item.retry_max_delay_secs) : "");
    setEscalationTiers(item.escalation_tiers ? String(item.escalation_tiers) : "");
    setEscalationMinInterval(
      item.escalation_min_interval_secs
        ? String(item.escalation_min_interval_secs)
        : ""
    );
    setEnabled(item.enabled);
  };

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">
              {selected ? "Update policy" : "Create policy"}
            </CardTitle>
            <CardDescription>
              Configure digest, retry, and escalation behavior by scope.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="policy-scope-type">Scope type</Label>
                <Input
                  disabled={!!selected}
                  id="policy-scope-type"
                  onChange={(event) =>
                    setScopeType(
                      (event.target.value || "project") as
                        | "project"
                        | "category"
                        | "workflow_step"
                    )
                  }
                  value={scopeType}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-scope-key">Scope key</Label>
                <Input
                  disabled={!!selected}
                  id="policy-scope-key"
                  onChange={(event) => setScopeKey(event.target.value)}
                  value={scopeKey}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-channel">Channel</Label>
                <Input
                  disabled={!!selected}
                  id="policy-channel"
                  onChange={(event) => setChannel(event.target.value)}
                  value={channel}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-digest">Digest policy</Label>
                <Input
                  id="policy-digest"
                  onChange={(event) => setDigestPolicy(event.target.value)}
                  value={digestPolicy}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-retry-attempts">Retry max attempts</Label>
                <Input
                  id="policy-retry-attempts"
                  onChange={(event) => setRetryAttempts(event.target.value)}
                  value={retryAttempts}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-retry-base">Retry base delay secs</Label>
                <Input
                  id="policy-retry-base"
                  onChange={(event) => setRetryBaseDelay(event.target.value)}
                  value={retryBaseDelay}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-retry-max">Retry max delay secs</Label>
                <Input
                  id="policy-retry-max"
                  onChange={(event) => setRetryMaxDelay(event.target.value)}
                  value={retryMaxDelay}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="policy-escalation-tiers">Escalation tiers</Label>
                <Input
                  id="policy-escalation-tiers"
                  onChange={(event) => setEscalationTiers(event.target.value)}
                  value={escalationTiers}
                />
              </div>
              <div className="space-y-1 md:col-span-2">
                <Label htmlFor="policy-escalation-interval">
                  Escalation min interval secs
                </Label>
                <Input
                  id="policy-escalation-interval"
                  onChange={(event) => setEscalationMinInterval(event.target.value)}
                  value={escalationMinInterval}
                />
              </div>
              <div className="space-y-1 md:col-span-2">
                <Label htmlFor="policy-enabled">Enabled</Label>
                <Input
                  id="policy-enabled"
                  onChange={(event) =>
                    setEnabled(event.target.value.toLowerCase() !== "false")
                  }
                  value={String(enabled)}
                />
              </div>
            </div>
            <div className="flex gap-2">
              <Button onClick={upsert}>{selected ? "Update" : "Create"}</Button>
              {selected ? (
                <>
                  <Button onClick={remove} variant="destructive">
                    Delete
                  </Button>
                  <Button onClick={resetForm} variant="outline">
                    Cancel
                  </Button>
                </>
              ) : null}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Policy overrides</CardTitle>
            <CardDescription>
              Click a policy to edit or remove it.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Scope</TableHead>
                  <TableHead>Scope key</TableHead>
                  <TableHead>Channel</TableHead>
                  <TableHead>Digest</TableHead>
                  <TableHead>Enabled</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedPolicies.length === 0 ? (
                  <TableRow>
                    <TableCell className="text-muted-foreground" colSpan={5}>
                      No policy overrides yet.
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedPolicies.map((item) => (
                    <TableRow
                      className="cursor-pointer"
                      key={item.id}
                      onClick={() => loadPolicyForEdit(item)}
                    >
                      <TableCell>{item.scope_type}</TableCell>
                      <TableCell>{item.scope_key}</TableCell>
                      <TableCell>{item.channel || "*"}</TableCell>
                      <TableCell>{item.digest_policy || "inherit"}</TableCell>
                      <TableCell>{item.enabled ? "yes" : "no"}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
