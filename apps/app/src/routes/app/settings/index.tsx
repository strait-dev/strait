import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { createFileRoute } from "@tanstack/react-router";
import { Suspense } from "react";
import Account from "@/components/(settings)/account";
import SubscriptionOverview from "@/components/(settings)/subscription-overview";
import { DefaultCatchBoundary } from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import PageHeader from "@/components/common/page-header";
import { CreditCardIcon, KeyIcon, UserIcon, UsersIcon } from "@/lib/icons";
import type { Session } from "@/routes/__root";

export const Route = createFileRoute("/app/settings/")({
  loader: ({ context }) => {
    const session = context.session as unknown as Session;
    if (!session) {
      throw new Error("Session unexpectedly null");
    }
    return {
      session,
    };
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

const MOCK_API_KEYS = [
  {
    id: "key_1",
    name: "Production API Key",
    prefix: "sk_live_****...X7kQ",
    scopes: ["read", "write"],
    createdAt: "2025-11-02T10:00:00Z",
    lastUsed: "2026-03-13T14:22:00Z",
  },
  {
    id: "key_2",
    name: "Development Key",
    prefix: "sk_test_****...m3Rp",
    scopes: ["read"],
    createdAt: "2026-01-15T08:30:00Z",
    lastUsed: "2026-03-10T09:15:00Z",
  },
  {
    id: "key_3",
    name: "CI/CD Pipeline",
    prefix: "sk_live_****...bN4w",
    scopes: ["read", "write", "admin"],
    createdAt: "2026-02-20T16:45:00Z",
    lastUsed: null,
  },
] as const;

const MOCK_TEAM_MEMBERS = [
  {
    id: "usr_1",
    name: "You",
    email: "you@example.com",
    role: "Owner",
    joinedAt: "2025-10-01T00:00:00Z",
  },
  {
    id: "usr_2",
    name: "Jane Smith",
    email: "jane@example.com",
    role: "Admin",
    joinedAt: "2025-11-15T00:00:00Z",
  },
  {
    id: "usr_3",
    name: "Bob Johnson",
    email: "bob@example.com",
    role: "Member",
    joinedAt: "2026-01-10T00:00:00Z",
  },
] as const;

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function ApiKeysTab() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>API Keys</CardTitle>
            <CardDescription>
              Manage API keys for programmatic access to your account.
            </CardDescription>
          </div>
          <Button size="sm">Create Key</Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <div className="rounded-md border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Name
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Key
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Scopes
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Created
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Last Used
                  </th>
                  <th
                    className="px-4 py-2 text-right font-medium text-muted-foreground"
                    scope="col"
                  />
                </tr>
              </thead>
              <tbody>
                {MOCK_API_KEYS.map((key) => (
                  <tr className="border-b last:border-b-0" key={key.id}>
                    <td className="px-4 py-3 font-medium">{key.name}</td>
                    <td className="px-4 py-3">
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                        {key.prefix}
                      </code>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        {key.scopes.map((scope) => (
                          <span
                            className="inline-flex rounded-full border px-2 py-0.5 font-medium text-muted-foreground text-xs"
                            key={scope}
                          >
                            {scope}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {formatDate(key.createdAt)}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {key.lastUsed ? formatDate(key.lastUsed) : "Never"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button size="sm" variant="destructive">
                        Revoke
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function TeamTab() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Team Members</CardTitle>
            <CardDescription>
              Manage who has access to your organization.
            </CardDescription>
          </div>
          <Button size="sm">Invite Member</Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <div className="rounded-md border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Member
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Role
                  </th>
                  <th
                    className="px-4 py-2 text-left font-medium text-muted-foreground"
                    scope="col"
                  >
                    Joined
                  </th>
                  <th
                    className="px-4 py-2 text-right font-medium text-muted-foreground"
                    scope="col"
                  />
                </tr>
              </thead>
              <tbody>
                {MOCK_TEAM_MEMBERS.map((member) => (
                  <tr className="border-b last:border-b-0" key={member.id}>
                    <td className="px-4 py-3">
                      <div className="flex flex-col">
                        <span className="font-medium">{member.name}</span>
                        <span className="text-muted-foreground text-xs">
                          {member.email}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex rounded-full border px-2 py-0.5 font-medium text-muted-foreground text-xs">
                        {member.role}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {formatDate(member.joinedAt)}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {member.role !== "Owner" && (
                        <Button size="sm" variant="outline">
                          Remove
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function RouteComponent() {
  const { session } = Route.useLoaderData();

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <PageHeader
          text="Manage your account, API keys, and team."
          title="Settings"
        />

        <Tabs className="w-full" defaultValue="account">
          <TabsList>
            <TabsTrigger className="flex items-center gap-2" value="account">
              <HugeiconsIcon className="size-4" icon={UserIcon} />
              Account
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              value="subscription"
            >
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Subscription
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="api-keys">
              <HugeiconsIcon className="size-4" icon={KeyIcon} />
              API Keys
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="team">
              <HugeiconsIcon className="size-4" icon={UsersIcon} />
              Team
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="account">
            <Account user={session.user} />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="subscription">
            <Suspense
              fallback={
                <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
                  Loading subscription...
                </div>
              }
            >
              <SubscriptionOverview />
            </Suspense>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="api-keys">
            <ApiKeysTab />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="team">
            <TeamTab />
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
