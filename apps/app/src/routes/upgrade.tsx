import { Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Polar } from "@polar-sh/sdk";
import { Button } from "@strait/ui/components/button";
import { Progress } from "@strait/ui/components/progress";
import { toast } from "@strait/ui/toast";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback, useEffect, useRef, useState } from "react";
import { useHotkeys } from "react-hotkeys-hook";
import * as z from "zod";
import {
  PlanSelection,
  type UpgradeMode,
} from "@/components/upgrade/plan-selection";
import { auth } from "@/lib/auth";
import { authMiddleware } from "@/middlewares/auth";
import type { AuthUser } from "@/routes/__root";
import { useUpgradeStore } from "@/stores/upgrade";

const POLAR_ACTIVE_STATUSES = new Set([
  "active",
  "trialing",
  "past_due",
  "incomplete",
  "unpaid",
]);

const PLAN_SLUGS: Record<string, string> = {
  "starter-monthly": "starter-monthly",
  "starter-yearly": "starter-yearly",
  "professional-monthly": "professional-monthly",
  "professional-yearly": "professional-yearly",
};

const polarClient = new Polar({
  accessToken: process.env.POLAR_ACCESS_TOKEN ?? "",
  server:
    (process.env.POLAR_SERVER as "sandbox" | "production") ?? "production",
});

const upgradeSearchSchema = z.object({
  checkout_success: z.coerce.boolean().optional(),
});

const getAuthUserFn = createServerFn({ method: "GET" }).handler(async () => {
  try {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    return (session?.user as AuthUser) ?? null;
  } catch {
    return null;
  }
});

const checkSubscriptionFn = createServerFn({ method: "GET" }).handler(
  async () => {
    try {
      const headers = getRequestHeaders();
      const session = await auth.api.getSession({ headers });

      if (!session?.user.email) {
        return false;
      }

      const { result: customersResult } = await polarClient.customers.list({
        email: session.user.email,
        limit: 1,
      });

      const customer = customersResult.items[0];
      if (!customer) {
        return false;
      }

      const { result: subscriptionsResult } =
        await polarClient.subscriptions.list({
          customerId: customer.id,
          limit: 20,
        });

      return subscriptionsResult.items.some((subscription) =>
        POLAR_ACTIVE_STATUSES.has(subscription.status)
      );
    } catch {
      return false;
    }
  }
);

type StartCheckoutInput = {
  planSlug: "starter" | "growth" | "professional" | "enterprise";
  billingInterval: "monthly" | "yearly";
};

const startCheckoutInputSchema = z.object({
  planSlug: z.enum(["starter", "growth", "professional", "enterprise"]),
  billingInterval: z.enum(["monthly", "yearly"]),
});

/**
 * Server function to start checkout for upgrade page.
 * Creates a Better Auth Polar checkout URL for the selected plan.
 */
const startCheckoutServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: StartCheckoutInput) =>
    startCheckoutInputSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(({ data }) => {
    const productSlug = `${data.planSlug}-${data.billingInterval}`;
    const checkoutProductSlug = PLAN_SLUGS[productSlug];

    if (!checkoutProductSlug) {
      throw new Error(`Invalid plan: ${productSlug}`);
    }

    const authBaseUrl =
      process.env.BETTER_AUTH_URL ??
      process.env.VITE_BASE_URL ??
      "http://localhost:5173";

    return {
      checkoutUrl: `${authBaseUrl}/api/auth/checkout/${checkoutProductSlug}`,
    };
  });

export const Route = createFileRoute("/upgrade")({
  validateSearch: zodValidator(upgradeSearchSchema),
  beforeLoad: async ({ context }) => {
    if (!context.isAuthenticated) {
      throw redirect({ to: "/login" });
    }

    const authUser = await getAuthUserFn();

    if (!authUser) {
      throw redirect({ to: "/login" });
    }

    const upgradeMode: UpgradeMode = authUser.defaultOrganizationId
      ? "checkout_recovery"
      : "new_user";

    return {
      upgradeMode: upgradeMode as UpgradeMode,
    };
  },
  component: UpgradePage,
});

function UpgradePage() {
  const { upgradeMode } = Route.useRouteContext();
  const search = Route.useSearch();
  const navigate = useNavigate();
  const { selectedPlan, billingInterval, reset } = useUpgradeStore();
  const [isProcessing, setIsProcessing] = useState(false);
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!search.checkout_success) {
      return;
    }
    setIsProcessing(true);

    const poll = async () => {
      try {
        const [user, hasSubscription] = await Promise.all([
          getAuthUserFn(),
          checkSubscriptionFn(),
        ]);

        if (hasSubscription || user?.defaultOrganizationId) {
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
          }
          navigate({ to: "/app", search: { trial_started: true } });
        }
      } catch {
        /* transient error — keep polling */
      }
    };

    poll();
    pollingRef.current = setInterval(poll, 2000);

    const timeout = setTimeout(() => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
      }
      setIsProcessing(false);
      toast.error(
        "Subscription processing is taking longer than expected. Please refresh the page."
      );
    }, 30_000);

    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
      }
      clearTimeout(timeout);
    };
  }, [search.checkout_success, navigate]);

  useEffect(() => {
    if (!search.checkout_success) {
      reset();
    }
  }, [reset, search.checkout_success]);

  const startCheckout = useMutation({
    mutationFn: () =>
      startCheckoutServerFn({
        data: {
          planSlug: selectedPlan,
          billingInterval,
        },
      }),
    onSuccess: (data) => {
      if (data.checkoutUrl) {
        window.location.assign(data.checkoutUrl);
      }
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : "Failed to start checkout"
      );
    },
  });

  const handleStartCheckout = useCallback(() => {
    startCheckout.mutate();
  }, [startCheckout]);

  // Keyboard shortcut for starting checkout
  useHotkeys(
    "mod+enter",
    () => {
      if (!startCheckout.isPending) {
        handleStartCheckout();
      }
    },
    { enableOnFormTags: true },
    [startCheckout.isPending, handleStartCheckout]
  );

  const buttonText =
    (upgradeMode as UpgradeMode) === "trial_ended"
      ? "Subscribe now"
      : "Start free trial";

  if (isProcessing) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <HugeiconsIcon
            className="h-8 w-8 animate-spin text-primary"
            icon={Loading03Icon}
          />
          <h2 className="font-semibold text-lg">
            Setting up your subscription...
          </h2>
          <p className="text-muted-foreground text-sm">
            This usually takes a few seconds.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="fixed top-0 right-0 left-0 z-30 border-border border-b bg-background">
        <div className="relative flex h-16 items-center justify-center px-4">
          <img
            alt="Strait Logo"
            className="h-8 w-auto"
            height={32}
            loading="eager"
            src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9FHxThgYVE9uncAThKs1v37lk5QJHeDdzbPmr"
            width={120}
          />
        </div>

        <Progress
          aria-label="Choose your plan"
          className="h-0.5 rounded-none"
          value={100}
        />
      </header>

      <main className="mt-[66px] mb-20 flex-1 overflow-auto">
        <div className="container mx-auto px-4 py-6">
          <div className="mx-auto max-w-5xl">
            <PlanSelection
              isLoading={startCheckout.isPending}
              mode={upgradeMode}
              onStartCheckout={handleStartCheckout}
            />
          </div>
        </div>
      </main>

      <footer className="fixed right-0 bottom-0 left-0 z-30 border-border border-t bg-background">
        <div className="container mx-auto px-4 py-4">
          <div className="mx-auto flex max-w-5xl items-center gap-3">
            <Button
              aria-label={buttonText}
              className="flex-1 gap-2"
              disabled={startCheckout.isPending}
              onClick={handleStartCheckout}
            >
              {startCheckout.isPending ? (
                <>
                  <HugeiconsIcon
                    className="h-4 w-4 animate-spin"
                    icon={Loading03Icon}
                  />
                  <span>Processing...</span>
                </>
              ) : (
                <>
                  {buttonText}
                  <kbd className="hidden rounded bg-primary-foreground/20 px-1.5 py-0.5 font-mono text-primary-foreground text-xs sm:inline-block">
                    ⌘↵
                  </kbd>
                </>
              )}
            </Button>
          </div>
        </div>
      </footer>
    </div>
  );
}
