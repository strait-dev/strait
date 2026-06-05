import { Separator } from "@strait/ui/components/separator";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@strait/ui/components/sidebar";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useEffect, useRef } from "react";
import * as z from "zod";
import PaymentStatusBanner from "@/components/billing/payment-status-banner";
import UpgradeNudgeBanner from "@/components/billing/upgrade-nudge-banner";
import ErrorComponent from "@/components/common/error-component";
import HeaderBreadcrumb from "@/components/common/header-breadcrumb";
import HeaderUserMenu from "@/components/common/header-user-menu";
import Sidebar from "@/components/common/sidebar";
import ThemeToggle from "@/components/common/theme-toggle";
import FeedbackDialog from "@/components/help/feedback-dialog";
import SupportDialog from "@/components/help/support-dialog";
import { usePostHog } from "@/components/providers/posthog-provider";
import { projectsQueryOptions } from "@/hooks/api/use-projects";
import {
  organizationQueryOptions,
  organizationsQueryOptions,
} from "@/hooks/auth/use-organization";
import {
  subscriptionQueryOptions,
  subscriptionStateQueryOptions,
} from "@/hooks/subscription/use-subscription";
import { ensureSession } from "@/lib/auth-handler";
import { setSentryUser } from "@/lib/sentry";
import { consumeUtmParams, utmToSetOnce } from "@/lib/utm";
import type { AuthUser, RouterContext, Session } from "@/routes/__root";

export type AppRouteContext = RouterContext & {
  session: NonNullable<Session>;
};

const appSearchSchema = z.object({
  checkout_success: z.coerce.boolean().optional(),
});

export const Route = createFileRoute("/app")({
  validateSearch: zodValidator(appSearchSchema),
  beforeLoad: async ({ context, location }) => {
    if (!context.isAuthenticated) {
      throw redirect({
        to: "/login",
        search: {
          redirect: location.href,
        },
      });
    }

    const sessionData = await ensureSession().catch(() => null);

    if (!sessionData) {
      throw redirect({
        to: "/login",
        search: { redirect: location.href },
      });
    }

    const session: NonNullable<Session> = {
      user: sessionData.user as AuthUser,
      session: sessionData.session,
    };

    // Legacy users who signed up before auto-workspace creation may not
    // have an organization yet. The user.create.after hook handles new
    // signups; this path only runs for pre-existing accounts.

    return { session };
  },
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    if (!session) {
      throw new Error("Session unexpectedly null in loader");
    }

    setSentryUser(session);

    const defaultOrgId = session.user.defaultOrganizationId;

    const activeProjectId = session.user.activeProjectId;

    let hasOrganization = !!defaultOrgId;

    const results = await Promise.allSettled([
      context.queryClient.ensureQueryData(organizationsQueryOptions()),
      defaultOrgId
        ? context.queryClient.ensureQueryData(
            organizationQueryOptions(defaultOrgId)
          )
        : Promise.resolve(),
      defaultOrgId
        ? context.queryClient.ensureQueryData(
            projectsQueryOptions(defaultOrgId)
          )
        : Promise.resolve(),
      context.queryClient.ensureQueryData(subscriptionQueryOptions()),
      context.queryClient.ensureQueryData(subscriptionStateQueryOptions()),
    ]);

    // If the org query failed, mark organization as unavailable
    if (results[1].status === "rejected") {
      hasOrganization = false;
    }

    return {
      session,
      hasOrganization,
      hasProject: hasOrganization && !!activeProjectId,
    };
  },
  errorComponent: ErrorComponent,
  component: RouteComponent,
});

function RouteComponent() {
  const { session } = Route.useLoaderData();
  const posthog = usePostHog();
  const hasIdentifiedRef = useRef(false);

  const { data: subscription } = useQuery(subscriptionQueryOptions());
  const { data: subscriptionState } = useQuery(subscriptionStateQueryOptions());

  useEffect(() => {
    if (
      hasIdentifiedRef.current ||
      !posthog ||
      !session?.user?.id ||
      !subscriptionState
    ) {
      return;
    }

    const plan = subscriptionState.planSlug ?? "none";
    const isTrialing = subscriptionState.isTrialing ?? false;
    const trialEnd = subscriptionState.trialInfo?.trialEnd ?? null;
    const organizationId = session.user.defaultOrganizationId;

    posthog.identify(session.user.id, {
      email: session.user.email,
      name: session.user.name || undefined,
      plan,
      is_trialing: isTrialing,
      trial_ends_at: trialEnd,
      organization_id: organizationId || undefined,
    });

    if (organizationId) {
      posthog.group("organization", organizationId, {
        plan,
        is_trialing: isTrialing,
        subscription_status: subscription?.status || "none",
      });
    }

    const utm = consumeUtmParams();
    const setOnce: Record<string, string> = {
      initial_signup_date: new Date(session.user.createdAt).toISOString(),
      ...(utm ? utmToSetOnce(utm) : {}),
    };
    posthog.setPersonProperties({}, setOnce);

    hasIdentifiedRef.current = true;
  }, [posthog, session, subscription, subscriptionState]);

  useEffect(() => {
    if (!(posthog && hasIdentifiedRef.current && subscriptionState)) {
      return;
    }

    const plan = subscriptionState.planSlug ?? "none";
    const isTrialing = subscriptionState.isTrialing ?? false;
    const trialEnd = subscriptionState.trialInfo?.trialEnd ?? null;

    posthog.setPersonProperties({
      plan,
      is_trialing: isTrialing,
      trial_ends_at: trialEnd,
    });

    const organizationId = session.user.defaultOrganizationId;
    if (organizationId) {
      posthog.group("organization", organizationId, {
        plan,
        is_trialing: isTrialing,
        subscription_status: subscription?.status || "none",
      });
    }
  }, [posthog, session, subscription, subscriptionState]);

  return (
    <SidebarProvider>
      <Sidebar session={session} />
      <SidebarInset>
        <div className="sticky top-0 z-30 shrink-0">
          <header className="flex h-16 items-center">
            <div className="w-full px-2">
              <div className="flex w-full items-center justify-between">
                <div className="flex items-center gap-3">
                  <SidebarTrigger className="-ml-1 text-muted-foreground group-data-[active=true]/menu-button:text-primary" />
                  <HeaderBreadcrumb />
                </div>
                <div className="flex items-center gap-1 sm:gap-2">
                  <ThemeToggle />
                  <span className="hidden sm:inline-flex">
                    <FeedbackDialog user={session.user} />
                  </span>
                  <SupportDialog user={session.user} />
                  <HeaderUserMenu user={session.user} />
                </div>
              </div>
            </div>
          </header>
          <Separator />
        </div>
        <div className="flex flex-1 flex-col gap-4 pt-0" vaul-drawer-wrapper="">
          <div className="space-y-2 px-4 pt-2">
            <PaymentStatusBanner />
            <UpgradeNudgeBanner />
          </div>
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
