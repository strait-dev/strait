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
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import type {
  NotificationProvider,
  NotifyDeliveryChannel,
} from "@/hooks/api/types";
import {
  notifyProvidersQueryOptions,
  useCreateNotificationProvider,
  useDeleteNotificationProvider,
  useUpdateNotificationProvider,
} from "@/hooks/api/use-notify";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/providers")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(notifyProvidersQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyProvidersPage,
});

const providerChannelOptions: readonly NotifyDeliveryChannel[] = [
  "email",
] as const;
const providerTypeOptions = ["ses"] as const;

const toKnownChannel = (value: NotifyDeliveryChannel | undefined) =>
  providerChannelOptions.find((option) => option === value) ?? "email";

const toKnownProvider = (value: string | undefined) =>
  providerTypeOptions.find((option) => option === value) ?? "ses";

const defaultConfig = {
  region: "us-east-1",
  from_email: "noreply@example.com",
  configuration_set: "",
};

function NotifyProvidersPage() {
  const { hasProject, session } = Route.useLoaderData();

  const providersQuery = useQuery({
    ...notifyProvidersQueryOptions(),
    enabled: hasProject,
  });

  const createProvider = useCreateNotificationProvider();
  const updateProvider = useUpdateNotificationProvider();
  const deleteProvider = useDeleteNotificationProvider();

  const [selected, setSelected] = useState<NotificationProvider | null>(null);

  const [channel, setChannel] =
    useState<(typeof providerChannelOptions)[number]>("email");
  const [provider, setProvider] =
    useState<(typeof providerTypeOptions)[number]>("ses");
  const [name, setName] = useState("");
  const [configJSON, setConfigJSON] = useState(
    JSON.stringify(defaultConfig, null, 2)
  );
  const [isDefault, setIsDefault] = useState(true);
  const [rateLimit, setRateLimit] = useState("");

  const providers = providersQuery.data ?? [];

  const sortedProviders = useMemo(
    () =>
      [...providers].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [providers]
  );

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const parseConfig = () => {
    try {
      return JSON.parse(configJSON) as Record<string, object>;
    } catch {
      return null;
    }
  };

  const parseRateLimit = () => {
    if (!rateLimit.trim()) {
      return undefined;
    }

    const parsed = Number(rateLimit);
    if (Number.isNaN(parsed) || parsed < 0) {
      return null;
    }

    return parsed;
  };

  const resetForm = () => {
    setSelected(null);
    setChannel("email");
    setProvider("ses");
    setName("");
    setConfigJSON(JSON.stringify(defaultConfig, null, 2));
    setIsDefault(true);
    setRateLimit("");
  };

  const loadProviderForEdit = (item: NotificationProvider) => {
    setSelected(item);
    setChannel(toKnownChannel(item.channel));
    setProvider(toKnownProvider(item.provider));
    setName(item.name);
    setIsDefault(item.is_default);
    setRateLimit(item.rate_limit ? String(item.rate_limit) : "");
    setConfigJSON(JSON.stringify(item.config ?? defaultConfig, null, 2));
  };

  const upsert = async () => {
    const config = parseConfig();
    if (!config) {
      toast.error("Config must be valid JSON");
      return;
    }

    const parsedRateLimit = parseRateLimit();
    if (parsedRateLimit === null) {
      toast.error("Rate limit must be a non-negative number");
      return;
    }

    const payload = {
      channel,
      provider,
      name: name.trim() || `${provider.toUpperCase()} ${channel}`,
      config,
      is_default: isDefault,
      rate_limit: parsedRateLimit,
    };

    if (selected) {
      await toast.promise(
        updateProvider.mutateAsync({ ...payload, providerId: selected.id }),
        {
          loading: "Updating provider...",
          success: "Provider updated",
          error: "Failed to update provider",
        }
      );
    } else {
      await toast.promise(createProvider.mutateAsync(payload), {
        loading: "Creating provider...",
        success: "Provider created",
        error: "Failed to create provider",
      });
    }

    resetForm();
  };

  const remove = async () => {
    if (!selected) {
      return;
    }

    await toast.promise(
      deleteProvider.mutateAsync({ providerId: selected.id }),
      {
        loading: "Deleting provider...",
        success: "Provider deleted",
        error: "Failed to delete provider",
      }
    );

    resetForm();
  };

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">
              {selected ? "Update provider" : "Create provider"}
            </CardTitle>
            <CardDescription>
              Notify currently supports SES for email delivery.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="provider-channel">Channel</Label>
                <Select
                  onValueChange={(value) =>
                    setChannel(value as NotifyDeliveryChannel)
                  }
                  value={channel}
                >
                  <SelectTrigger id="provider-channel">
                    <SelectValue placeholder="Choose channel" />
                  </SelectTrigger>
                  <SelectContent>
                    {providerChannelOptions.map((value) => (
                      <SelectItem key={value} value={value}>
                        {value}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="provider-name">Provider</Label>
                <Select
                  onValueChange={(value) => setProvider(value as "ses")}
                  value={provider}
                >
                  <SelectTrigger id="provider-name">
                    <SelectValue placeholder="Choose provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {providerTypeOptions.map((value) => (
                      <SelectItem key={value} value={value}>
                        {value}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="display-name">Display name</Label>
              <Input
                id="display-name"
                onChange={(event) => setName(event.target.value)}
                value={name}
              />
            </div>

            <div className="space-y-1">
              <Label htmlFor="provider-rate-limit">Rate limit</Label>
              <Input
                id="provider-rate-limit"
                onChange={(event) => setRateLimit(event.target.value)}
                placeholder="optional"
                value={rateLimit}
              />
            </div>

            <div className="space-y-1">
              <Label htmlFor="provider-config">Config JSON</Label>
              <Textarea
                className="min-h-[180px] font-mono text-xs"
                id="provider-config"
                onChange={(event) => setConfigJSON(event.target.value)}
                value={configJSON}
              />
            </div>

            <div className="flex items-center justify-between rounded-md border p-3">
              <div>
                <p className="font-medium text-sm">Default provider</p>
                <p className="text-muted-foreground text-xs">
                  Used when no provider is explicitly selected.
                </p>
              </div>
              <Switch checked={isDefault} onCheckedChange={setIsDefault} />
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
            <CardTitle className="text-sm">Providers</CardTitle>
            <CardDescription>
              Click a provider to edit or remove it.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Channel</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Default</TableHead>
                  <TableHead>Health</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedProviders.length === 0 ? (
                  <TableRow>
                    <TableCell className="text-muted-foreground" colSpan={5}>
                      No providers yet.
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedProviders.map((item) => (
                    <TableRow
                      className="cursor-pointer"
                      key={item.id}
                      onClick={() => loadProviderForEdit(item)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          loadProviderForEdit(item);
                        }
                      }}
                      role="button"
                      tabIndex={0}
                    >
                      <TableCell>{item.name}</TableCell>
                      <TableCell>{item.channel}</TableCell>
                      <TableCell>{item.provider}</TableCell>
                      <TableCell>{item.is_default ? "yes" : "no"}</TableCell>
                      <TableCell>{item.health || "unknown"}</TableCell>
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
