import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { Switch } from "@strait/ui/components/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import type { NotifyPreference } from "@/hooks/api/types";
import {
  createNotifySubscriberTokenFn,
  notifySubscriberPreferencesQueryOptions,
  notifySubscriberQueryOptions,
  notifySubscriberSuppressionsQueryOptions,
  notifyTopicsQueryOptions,
  updateNotifySubscriberFn,
  useAddNotifyTopicSubscriber,
  useNotifyUnsuppressSubscriber,
  useRemoveNotifyTopicSubscriber,
  useUpdateNotifySubscriberPreference,
} from "@/hooks/api/use-notify";
import { CheckIcon, ChevronLeftIcon, KeyIcon } from "@/lib/icons";

const digestPolicyOptions = ["instant", "hourly", "daily"] as const;

const parseChannelPref = (
  preference: NotifyPreference | undefined,
  channel: "email" | "inbox",
  fallback: boolean
) => {
  if (
    !preference?.channel_prefs ||
    typeof preference.channel_prefs !== "object"
  ) {
    return fallback;
  }

  const value = (
    preference.channel_prefs as Record<
      string,
      object | string | number | boolean | null
    >
  )[channel];
  if (typeof value === "boolean") {
    return value;
  }

  return fallback;
};

const parseDigestPolicy = (preference: NotifyPreference | undefined) => {
  const value = preference?.digest_policy;
  return digestPolicyOptions.find((option) => option === value) ?? "instant";
};

export const Route = createFileRoute("/app/notify/subscribers/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(
        notifySubscriberQueryOptions(params.id)
      ),
      context.queryClient.ensureQueryData(
        notifySubscriberSuppressionsQueryOptions(params.id)
      ),
      context.queryClient.ensureQueryData(
        notifySubscriberPreferencesQueryOptions(params.id)
      ),
      context.queryClient.ensureQueryData(notifyTopicsQueryOptions()),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifySubscriberDetailPage,
});

function NotifySubscriberDetailPage() {
  const { id } = Route.useParams();

  const subscriberQuery = useQuery(notifySubscriberQueryOptions(id));
  const suppressionsQuery = useQuery(
    notifySubscriberSuppressionsQueryOptions(id)
  );
  const preferencesQuery = useQuery(
    notifySubscriberPreferencesQueryOptions(id)
  );
  const topicsQuery = useQuery(notifyTopicsQueryOptions());

  const [externalID, setExternalID] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [locale, setLocale] = useState("");
  const [timezone, setTimezone] = useState("");
  const [tenantID, setTenantID] = useState("");

  const [unsuppressReason, setUnsuppressReason] = useState("manual_unsuppress");
  const [forceUnsuppress, setForceUnsuppress] = useState(false);
  const [topicMembershipKey, setTopicMembershipKey] = useState("");

  const [preferenceScope, setPreferenceScope] = useState("global");
  const [prefEmailEnabled, setPrefEmailEnabled] = useState(true);
  const [prefInboxEnabled, setPrefInboxEnabled] = useState(true);
  const [prefTimezone, setPrefTimezone] = useState("");
  const [prefDigestPolicy, setPrefDigestPolicy] =
    useState<(typeof digestPolicyOptions)[number]>("instant");
  const [prefCriticalOverride, setPrefCriticalOverride] = useState(true);
  const [prefRateLimit, setPrefRateLimit] = useState("");

  const unsuppressMutation = useNotifyUnsuppressSubscriber();
  const updatePreferenceMutation = useUpdateNotifySubscriberPreference();
  const addTopicSubscriberMutation = useAddNotifyTopicSubscriber();
  const removeTopicSubscriberMutation = useRemoveNotifyTopicSubscriber();

  const subscriber = subscriberQuery.data;
  const suppressions = suppressionsQuery.data ?? [];
  const preferences = preferencesQuery.data ?? [];
  const topics = topicsQuery.data ?? [];

  const selectedPreference = useMemo(
    () => preferences.find((item) => item.scope === preferenceScope),
    [preferences, preferenceScope]
  );

  const preferenceScopes = useMemo(() => {
    const scopes = new Set<string>(["global", preferenceScope]);
    for (const item of preferences) {
      scopes.add(item.scope);
    }
    return Array.from(scopes);
  }, [preferences, preferenceScope]);

  useEffect(() => {
    if (!selectedPreference) {
      setPrefEmailEnabled(true);
      setPrefInboxEnabled(true);
      setPrefTimezone("");
      setPrefDigestPolicy("instant");
      setPrefCriticalOverride(true);
      setPrefRateLimit("");
      return;
    }

    setPrefEmailEnabled(parseChannelPref(selectedPreference, "email", true));
    setPrefInboxEnabled(parseChannelPref(selectedPreference, "inbox", true));
    setPrefTimezone(selectedPreference.timezone || "");
    setPrefDigestPolicy(parseDigestPolicy(selectedPreference));
    setPrefCriticalOverride(selectedPreference.critical_override);
    setPrefRateLimit(
      selectedPreference.rate_limit_override
        ? String(selectedPreference.rate_limit_override)
        : ""
    );
  }, [selectedPreference]);

  if (!subscriber) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/notify/subscribers" entity="Subscriber" />
      </Shell>
    );
  }

  const defaults = {
    externalID: externalID || subscriber.external_id,
    email: email || subscriber.email || "",
    phone: phone || subscriber.phone || "",
    locale: locale || subscriber.locale || "",
    timezone: timezone || subscriber.timezone || "",
    tenantID: tenantID || subscriber.tenant_id || "",
  };

  const saveSubscriber = async () => {
    await toast.promise(
      updateNotifySubscriberFn({
        data: {
          subscriberId: subscriber.id,
          external_id: defaults.externalID,
          email: defaults.email || undefined,
          phone: defaults.phone || undefined,
          locale: defaults.locale || undefined,
          timezone: defaults.timezone || undefined,
          tenant_id: defaults.tenantID || undefined,
        },
      }),
      {
        loading: "Updating subscriber...",
        success: "Subscriber updated",
        error: "Failed to update subscriber",
      }
    );

    await subscriberQuery.refetch();
  };

  const savePreference = async () => {
    if (!preferenceScope.trim()) {
      toast.error("Preference scope is required");
      return;
    }

    let parsedRateLimit: number | undefined;
    if (prefRateLimit.trim()) {
      const parsed = Number(prefRateLimit);
      if (Number.isNaN(parsed) || parsed < 0) {
        toast.error("Rate limit override must be a non-negative number");
        return;
      }
      parsedRateLimit = parsed;
    }

    await toast.promise(
      updatePreferenceMutation.mutateAsync({
        subscriberId: subscriber.id,
        scope: preferenceScope.trim(),
        channel_prefs: {
          email: prefEmailEnabled,
          inbox: prefInboxEnabled,
        },
        timezone: prefTimezone || undefined,
        digest_policy: prefDigestPolicy,
        critical_override: prefCriticalOverride,
        rate_limit_override: parsedRateLimit,
      }),
      {
        loading: "Updating preferences...",
        success: "Preferences updated",
        error: "Failed to update preferences",
      }
    );

    await preferencesQuery.refetch();
  };

  const updateTopicMembership = async (action: "add" | "remove") => {
    if (!topicMembershipKey.trim()) {
      toast.error("Topic key is required");
      return;
    }

    const promise =
      action === "add"
        ? addTopicSubscriberMutation.mutateAsync({
            topicKey: topicMembershipKey.trim(),
            subscriber_id: subscriber.id,
          })
        : removeTopicSubscriberMutation.mutateAsync({
            topicKey: topicMembershipKey.trim(),
            subscriberId: subscriber.id,
          });

    await toast.promise(promise, {
      loading:
        action === "add"
          ? "Adding subscriber to topic..."
          : "Removing subscriber from topic...",
      success:
        action === "add"
          ? "Subscriber added to topic"
          : "Subscriber removed from topic",
      error:
        action === "add"
          ? "Failed to add subscriber to topic"
          : "Failed to remove subscriber from topic",
    });

    await topicsQuery.refetch();
  };

  const unsuppress = async () => {
    await toast.promise(
      unsuppressMutation.mutateAsync({
        subscriberId: subscriber.id,
        channel: "email",
        reason: unsuppressReason,
        force: forceUnsuppress,
      }),
      {
        loading: "Unsuppressing email channel...",
        success: "Unsuppress request applied",
        error: "Failed to unsuppress",
      }
    );

    await Promise.all([subscriberQuery.refetch(), suppressionsQuery.refetch()]);
  };

  const createToken = async () => {
    await toast.promise(
      createNotifySubscriberTokenFn({
        data: { subscriberId: subscriber.id, expires_in: "24h" },
      }).then((result) => {
        navigator.clipboard.writeText(result.token).catch(() => null);
      }),
      {
        loading: "Creating token...",
        success: "Token copied to clipboard",
        error: "Failed to create token",
      }
    );
  };

  return (
    <Shell>
      <div className="mb-4 flex items-center gap-3">
        <Button
          render={<Link to="/app/notify/subscribers" />}
          size="sm"
          variant="ghost"
        >
          <HugeiconsIcon icon={ChevronLeftIcon} size={16} />
        </Button>
        <div>
          <h1 className="font-semibold text-xl">{subscriber.external_id}</h1>
          <div className="mt-1 flex items-center gap-2">
            <NotifyStatusBadge status={subscriber.status} />
            <span className="text-muted-foreground text-xs">
              {subscriber.id}
            </span>
          </div>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Subscriber profile</CardTitle>
            <CardDescription>
              Update routing fields used by Notify APIs.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="external-id">External ID</Label>
                <Input
                  id="external-id"
                  onChange={(event) => setExternalID(event.target.value)}
                  placeholder="external-id"
                  value={defaults.externalID}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="tenant-id">Tenant ID</Label>
                <Input
                  id="tenant-id"
                  onChange={(event) => setTenantID(event.target.value)}
                  placeholder="tenant"
                  value={defaults.tenantID}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="email@example.com"
                  value={defaults.email}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="phone">Phone</Label>
                <Input
                  id="phone"
                  onChange={(event) => setPhone(event.target.value)}
                  placeholder="+1..."
                  value={defaults.phone}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="locale">Locale</Label>
                <Input
                  id="locale"
                  onChange={(event) => setLocale(event.target.value)}
                  placeholder="en"
                  value={defaults.locale}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="timezone">Timezone</Label>
                <Input
                  id="timezone"
                  onChange={(event) => setTimezone(event.target.value)}
                  placeholder="UTC"
                  value={defaults.timezone}
                />
              </div>
            </div>
            <div className="flex gap-2">
              <Button onClick={saveSubscriber}>
                <HugeiconsIcon className="mr-1.5 size-4" icon={CheckIcon} />
                Save
              </Button>
              <Button onClick={createToken} variant="outline">
                <HugeiconsIcon className="mr-1.5 size-4" icon={KeyIcon} />
                Create token
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Preference controls</CardTitle>
            <CardDescription>
              Manage per-scope channel and digest preferences for this
              subscriber.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="preference-scope">Scope</Label>
              <Input
                id="preference-scope"
                list="notify-preference-scopes"
                onChange={(event) => setPreferenceScope(event.target.value)}
                placeholder="global"
                value={preferenceScope}
              />
              <datalist id="notify-preference-scopes">
                {preferenceScopes.map((scope) => (
                  <option key={scope} value={scope} />
                ))}
              </datalist>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="flex items-center justify-between rounded-md border p-3">
                <div>
                  <p className="font-medium text-sm">Email channel</p>
                  <p className="text-muted-foreground text-xs">
                    Enable email delivery
                  </p>
                </div>
                <Switch
                  checked={prefEmailEnabled}
                  onCheckedChange={setPrefEmailEnabled}
                />
              </div>

              <div className="flex items-center justify-between rounded-md border p-3">
                <div>
                  <p className="font-medium text-sm">Inbox channel</p>
                  <p className="text-muted-foreground text-xs">
                    Enable inbox delivery
                  </p>
                </div>
                <Switch
                  checked={prefInboxEnabled}
                  onCheckedChange={setPrefInboxEnabled}
                />
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="preference-timezone">Timezone override</Label>
                <Input
                  id="preference-timezone"
                  onChange={(event) => setPrefTimezone(event.target.value)}
                  placeholder="UTC"
                  value={prefTimezone}
                />
              </div>

              <div className="space-y-1">
                <Label htmlFor="preference-digest">Digest policy</Label>
                <Select
                  onValueChange={(value) =>
                    setPrefDigestPolicy(
                      value as (typeof digestPolicyOptions)[number]
                    )
                  }
                  value={prefDigestPolicy}
                >
                  <SelectTrigger id="preference-digest">
                    <SelectValue placeholder="Choose digest policy" />
                  </SelectTrigger>
                  <SelectContent>
                    {digestPolicyOptions.map((option) => (
                      <SelectItem key={option} value={option}>
                        {option}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="flex items-center justify-between rounded-md border p-3">
                <div>
                  <p className="font-medium text-sm">Critical override</p>
                  <p className="text-muted-foreground text-xs">
                    Allow critical notifications to bypass quieting rules
                  </p>
                </div>
                <Switch
                  checked={prefCriticalOverride}
                  onCheckedChange={setPrefCriticalOverride}
                />
              </div>

              <div className="space-y-1">
                <Label htmlFor="preference-rate-limit">
                  Rate limit override
                </Label>
                <Input
                  id="preference-rate-limit"
                  onChange={(event) => setPrefRateLimit(event.target.value)}
                  placeholder="optional"
                  value={prefRateLimit}
                />
              </div>
            </div>

            <Button onClick={savePreference}>Save preferences</Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Topic membership</CardTitle>
            <CardDescription>
              Add or remove this subscriber from a notify topic.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="subscriber-topic-membership">Topic key</Label>
              <Input
                id="subscriber-topic-membership"
                list="subscriber-topic-membership-options"
                onChange={(event) => setTopicMembershipKey(event.target.value)}
                placeholder="workflow.approvals"
                value={topicMembershipKey}
              />
              <datalist id="subscriber-topic-membership-options">
                {topics.map((topic) => (
                  <option key={topic.id} value={topic.topic_key}>
                    {topic.name}
                  </option>
                ))}
              </datalist>
            </div>

            <div className="flex gap-2">
              <Button onClick={() => updateTopicMembership("add")}>Add</Button>
              <Button
                onClick={() => updateTopicMembership("remove")}
                variant="outline"
              >
                Remove
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Suppression controls</CardTitle>
            <CardDescription>
              Unsuppress email channel when policy allows. Use force for
              complaint/bounce cases.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="reason">Reason</Label>
              <Textarea
                id="reason"
                onChange={(event) => setUnsuppressReason(event.target.value)}
                value={unsuppressReason}
              />
            </div>
            <div className="flex items-center justify-between rounded-md border p-3">
              <div>
                <p className="font-medium text-sm">Force unsuppress</p>
                <p className="text-muted-foreground text-xs">
                  Required for complaint/bounce suppressions.
                </p>
              </div>
              <Switch
                checked={forceUnsuppress}
                onCheckedChange={setForceUnsuppress}
              />
            </div>
            <Button onClick={unsuppress}>Unsuppress email</Button>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-sm">Suppression history</CardTitle>
          <CardDescription>
            Latest suppression lifecycle events for this subscriber.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Action</TableHead>
                <TableHead>Channel</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {suppressions.length === 0 ? (
                <TableRow>
                  <TableCell className="text-muted-foreground" colSpan={5}>
                    No suppression events
                  </TableCell>
                </TableRow>
              ) : (
                suppressions.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell>
                      <Badge variant="secondary">{event.action}</Badge>
                    </TableCell>
                    <TableCell>{event.channel}</TableCell>
                    <TableCell>{event.reason || "-"}</TableCell>
                    <TableCell>{event.source}</TableCell>
                    <TableCell>
                      {new Date(event.created_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </Shell>
  );
}
