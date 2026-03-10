import { Crown03Icon, SquareLock02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useCallback, useMemo } from "react";
import { FEATURE_FLAGS, type FeatureFlagKey } from "@/hooks/posthog/flags";
import { useFeatureFlag } from "@/hooks/posthog/use-feature-flag";
import type { Session } from "@/routes/__root";

/**
 * Feature keys that can be used with FeatureProtectedRoute.
 * Maps to PostHog feature flag keys.
 */
export type ProtectedFeature =
  | "stockTransfers"
  | "returns"
  | "stockCounts"
  | "stockControl"
  | "quotes"
  | "storeCredit"
  | "giftCards"
  | "aiAssistant"
  | "aiSalesAgent"
  | "aiReportsAgent"
  | "aiInventoryAgent"
  | "aiMarketingAgent"
  | "registerSummaries"
  | "employeeSalesTracking";

const FEATURE_TO_FLAG_MAP: Record<ProtectedFeature, FeatureFlagKey> = {
  stockTransfers: FEATURE_FLAGS.STOCK_TRANSFERS,
  returns: FEATURE_FLAGS.RETURNS,
  stockCounts: FEATURE_FLAGS.STOCK_COUNTS,
  stockControl: FEATURE_FLAGS.STOCK_COUNTS, // stockControl maps to stock_counts for route protection
  quotes: FEATURE_FLAGS.QUOTES,
  storeCredit: FEATURE_FLAGS.STORE_CREDIT,
  giftCards: FEATURE_FLAGS.GIFT_CARDS,
  aiAssistant: FEATURE_FLAGS.AI_ASSISTANT,
  aiSalesAgent: FEATURE_FLAGS.AI_SALES_AGENT,
  aiReportsAgent: FEATURE_FLAGS.AI_REPORTS_AGENT,
  aiInventoryAgent: FEATURE_FLAGS.AI_INVENTORY_AGENT,
  aiMarketingAgent: FEATURE_FLAGS.AI_MARKETING_AGENT,
  registerSummaries: FEATURE_FLAGS.REGISTER_SUMMARIES,
  employeeSalesTracking: FEATURE_FLAGS.EMPLOYEE_SALES_TRACKING,
};

type FeatureProtectedRouteProps = {
  session: Session | null;
  feature: ProtectedFeature;
  children: ReactNode;
  featureName?: string;
  featureDescription?: string;
};

/**
 * Component that protects entire routes based on subscription features
 * Optimized with React hooks for better performance
 */
export const FeatureProtectedRoute = ({
  feature,
  children,
  featureName,
  featureDescription,
}: FeatureProtectedRouteProps) => {
  const navigate = useNavigate();

  // Get the PostHog flag key for this feature
  const flagKey = FEATURE_TO_FLAG_MAP[feature];
  const hasAccess = useFeatureFlag(flagKey);

  // Memoize feature display names to avoid recreation on every render
  const featureDisplayNames = useMemo(
    (): Record<ProtectedFeature, string> => ({
      stockTransfers: "Stock Transfers",
      returns: "Returns & Refunds",
      stockCounts: "Stock Counts",
      stockControl: "Stock Control",
      quotes: "Quote Management",
      storeCredit: "Store Credit",
      giftCards: "Gift Cards",
      aiAssistant: "AI Assistant",
      aiSalesAgent: "AI Sales Agent",
      aiReportsAgent: "AI Reports Agent",
      aiInventoryAgent: "AI Inventory Agent",
      aiMarketingAgent: "AI Marketing Agent",
      registerSummaries: "Register Summaries",
      employeeSalesTracking: "Employee Sales Tracking",
    }),
    []
  );

  // Memoize feature descriptions to avoid recreation on every render
  const featureDescriptions = useMemo(
    (): Record<ProtectedFeature, string> => ({
      stockTransfers:
        "Transfer inventory between locations and warehouses with full tracking and control.",
      returns:
        "Process customer returns and manage refunds efficiently with comprehensive tracking.",
      stockCounts:
        "Perform physical inventory counts and reconciliation to maintain accurate stock levels.",
      stockControl: "Level of inventory management automation.",
      quotes: "Create and manage customer quotes.",
      storeCredit: "Issue and manage store credit.",
      giftCards: "Sell and manage gift cards.",
      aiAssistant: "Get intelligent AI assistance for your business.",
      aiSalesAgent: "AI-powered sales recommendations and coaching.",
      aiReportsAgent: "Generate custom reports with AI.",
      aiInventoryAgent: "AI-powered inventory optimization.",
      aiMarketingAgent: "AI-driven marketing campaigns.",
      registerSummaries: "View register-specific summaries.",
      employeeSalesTracking: "Track employee sales performance.",
    }),
    []
  );

  // Memoize display name getter
  const getFeatureDisplayName = useCallback(
    (featureKey: ProtectedFeature): string =>
      featureDisplayNames[featureKey] || String(featureKey),
    [featureDisplayNames]
  );

  // Memoize description getter
  const getFeatureDescription = useCallback(
    (featureKey: ProtectedFeature): string =>
      featureDescriptions[featureKey] ||
      `Access ${String(featureKey)} functionality`,
    [featureDescriptions]
  );

  // Memoize navigation handlers
  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  const handleBackToDashboard = useCallback(() => {
    navigate({ to: "/app" });
  }, [navigate]);

  // Memoize fallback component to prevent unnecessary re-renders
  const fallbackComponent = useMemo(
    () => (
      <div className="flex min-h-[60vh] items-center justify-center p-8">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-accent">
              <HugeiconsIcon
                className="h-6 w-6 text-accent-foreground"
                icon={SquareLock02Icon}
              />
            </div>
            <CardTitle className="text-xl">
              {featureName || getFeatureDisplayName(feature)} Required
            </CardTitle>
            <CardDescription>
              {featureDescription || getFeatureDescription(feature)}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 text-center">
            <p className="text-muted-foreground text-sm">
              This feature requires a higher subscription tier. Upgrade your
              plan to access this functionality.
            </p>
            <Button className="w-full" onClick={handleUpgrade}>
              <HugeiconsIcon className="size-4" icon={Crown03Icon} />
              Upgrade Plan
            </Button>
            <Button
              className="w-full"
              onClick={handleBackToDashboard}
              variant="outline"
            >
              Back to Dashboard
            </Button>
          </CardContent>
        </Card>
      </div>
    ),
    [
      featureName,
      featureDescription,
      feature,
      getFeatureDisplayName,
      getFeatureDescription,
      handleUpgrade,
      handleBackToDashboard,
    ]
  );

  // Enable all features in development mode
  const isDevelopment = process.env.NODE_ENV === "development";
  if (isDevelopment) {
    return <>{children}</>;
  }

  if (hasAccess) {
    return children;
  }

  return fallbackComponent;
};
