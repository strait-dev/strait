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
import { Switch } from "@strait/ui/components/switch";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import Account from "@/components/(settings)/account";
import { AuthorizedApps } from "@/components/(settings)/authorized-apps";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import {
  emailPreferencesQueryOptions,
  useUpdateEmailPreferences,
} from "@/hooks/billing/use-email-preferences";
import { CreditCardIcon, LinkSquareIcon, UserIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/settings/")({
  head: () => ({ meta: [{ title: "Settings · Strait" }] }),
  loader: ({ context }) => {
    const { session } = context as AppRouteContext;
    return {
      session,
    };
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function EmailPreferencesCard() {
  const { data: prefs } = useQuery(emailPreferencesQueryOptions());
  const updatePrefs = useUpdateEmailPreferences();

  const monthlyEmail = prefs?.monthly_usage_email ?? true;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Email notifications</CardTitle>
        <CardDescription>
          Configure which billing emails you receive for your organization.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="flex flex-col gap-0.5">
            <span className="font-medium text-sm">Monthly usage report</span>
            <span className="text-muted-foreground text-xs">
              Receive a PDF usage summary email when your billing period ends.
            </span>
          </div>
          <Switch
            checked={monthlyEmail}
            disabled={updatePrefs.isPending}
            onCheckedChange={(checked: boolean) => {
              updatePrefs.mutate({ monthlyUsageEmail: checked });
            }}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function RouteComponent() {
  usePageEvent("settings_viewed");
  const { session } = Route.useLoaderData();

  return (
    <Shell>
      <h1 className="sr-only">Settings</h1>
      <div className="flex w-full flex-col gap-6">
        <Tabs className="w-full" defaultValue="account">
          <TabsList>
            <TabsTrigger className="flex items-center gap-2" value="account">
              <HugeiconsIcon className="size-4" icon={UserIcon} />
              Account
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="billing">
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Usage & Billing
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="apps">
              <HugeiconsIcon className="size-4" icon={LinkSquareIcon} />
              Authorized apps
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="account">
            <Account user={session.user} />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="apps">
            <AuthorizedApps />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="billing">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Usage & Billing</CardTitle>
                <CardDescription>
                  View detailed usage metrics, cost breakdowns, and spending
                  limits for your organization.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Button render={<Link to="/app/billing" />} variant="outline">
                  View billing details
                </Button>
              </CardContent>
            </Card>

            <EmailPreferencesCard />
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
