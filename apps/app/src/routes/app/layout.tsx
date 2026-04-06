import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@strait/ui/components/sidebar";
import { useQuery } from "@tanstack/react-query";
import {
  createFileRoute,
  Outlet,
  redirect,
  useNavigate,
} from "@tanstack/react-router";
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
import TrialStartedModal from "@/components/upgrade/trial-started-modal";
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
  trial_started: z.coerce.boolean().optional(),
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
  const search = Route.useSearch();
  const navigate = useNavigate();
  const posthog = usePostHog();
  const showTrialModal = Boolean(search.trial_started);
  const hasIdentifiedRef = useRef(false);

  const { data: subscription } = useQuery(subscriptionQueryOptions());
  const { data: subscriptionState } = useQuery(subscriptionStateQueryOptions());

  useEffect(() => {
    if (!(posthog && session?.user?.id && subscriptionState)) {
      return;
    }

    const plan = subscriptionState.planSlug ?? "none";
    const isTrialing = subscriptionState.isTrialing ?? false;
    const trialEnd = subscriptionState.trialInfo?.trialEnd ?? null;
    const organizationId = session.user.defaultOrganizationId;

    // First identification: set identity, UTM params, and initial properties.
    if (!hasIdentifiedRef.current) {
      posthog.identify(session.user.id, {
        email: session.user.email,
        name: session.user.name || undefined,
        plan,
        is_trialing: isTrialing,
        trial_ends_at: trialEnd,
        organization_id: organizationId || undefined,
      });

      const utm = consumeUtmParams();
      const setOnce: Record<string, string> = {
        initial_signup_date: new Date(session.user.createdAt).toISOString(),
        ...(utm ? utmToSetOnce(utm) : {}),
      };
      posthog.setPersonProperties({}, setOnce);
      hasIdentifiedRef.current = true;
    }

    // Always update plan properties when subscription changes.
    posthog.setPersonProperties({
      plan,
      is_trialing: isTrialing,
      trial_ends_at: trialEnd,
    });

    if (organizationId) {
      posthog.group("organization", organizationId, {
        plan,
        is_trialing: isTrialing,
        subscription_status: subscription?.status || "none",
      });
    }
  }, [posthog, session, subscription, subscriptionState]);

  const handleTrialModalClose = (open: boolean) => {
    if (!open) {
      navigate({
        to: "/app",
        search: {},
        replace: true,
      });
    }
  };

  return (
    <SidebarProvider>
      <Sidebar session={session} />
      <SidebarInset>
        <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center border-b bg-background">
          <div className="w-full px-2">
            <div className="flex w-full items-center justify-between">
              <div className="flex items-center gap-3">
                <SidebarTrigger className="-ml-1 text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary" />
                <HeaderBreadcrumb />
              </div>
              <div className="flex items-center gap-1 sm:gap-2">
                <span className="hidden sm:inline-flex">
                  <ThemeToggle />
                </span>
                <span className="hidden sm:inline-flex">
                  <FeedbackDialog user={session.user} />
                </span>
                <SupportDialog user={session.user} />
                <HeaderUserMenu user={session.user} />
              </div>
            </div>
          </div>
        </header>
        <div
          className="flex flex-1 flex-col gap-4 bg-background pt-0"
          vaul-drawer-wrapper=""
        >
          <div className="space-y-2 px-4 pt-2">
            <PaymentStatusBanner />
            <UpgradeNudgeBanner />
          </div>
          <Outlet />
        </div>
      </SidebarInset>

      <TrialStartedModal
        onOpenChange={handleTrialModalClose}
        open={showTrialModal}
      />
    </SidebarProvider>
  );
}
