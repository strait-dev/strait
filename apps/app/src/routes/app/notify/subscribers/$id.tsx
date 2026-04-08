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
import { useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import {
  createNotifySubscriberTokenFn,
  notifySubscriberQueryOptions,
  notifySubscriberSuppressionsQueryOptions,
  updateNotifySubscriberFn,
  useNotifyUnsuppressSubscriber,
} from "@/hooks/api/use-notify";
import { CheckIcon, ChevronLeftIcon, KeyIcon } from "@/lib/icons";

export const Route = createFileRoute("/app/notify/subscribers/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(
        notifySubscriberQueryOptions(params.id)
      ),
      context.queryClient.ensureQueryData(
        notifySubscriberSuppressionsQueryOptions(params.id)
      ),
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

  const [externalID, setExternalID] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [locale, setLocale] = useState("");
  const [timezone, setTimezone] = useState("");
  const [tenantID, setTenantID] = useState("");

  const [unsuppressReason, setUnsuppressReason] = useState("manual_unsuppress");
  const [forceUnsuppress, setForceUnsuppress] = useState(false);

  const unsuppressMutation = useNotifyUnsuppressSubscriber();

  const subscriber = subscriberQuery.data;
  const suppressions = suppressionsQuery.data ?? [];

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
