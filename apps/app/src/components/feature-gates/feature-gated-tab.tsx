import { HugeiconsIcon } from "@hugeicons/react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import { TabsContent, TabsTrigger } from "@strait/ui/components/tabs";
import { cn } from "@strait/ui/utils/index";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useCallback, useState } from "react";
import type { FeatureFlagKey } from "@/hooks/posthog/flags";
import { useFeatureFlag } from "@/hooks/posthog/use-feature-flag";
import { CrownIcon, LockSquareIcon } from "@/lib/icons";

type FeatureGatedTabTriggerProps = {
  value: string;
  flagKey: FeatureFlagKey;
  children: ReactNode;
  className?: string;
};

/**
 * A tab trigger that shows a lock icon when the feature is not available.
 * When clicked while locked, it shows an upgrade dialog.
 */
export const FeatureGatedTabTrigger = ({
  value,
  flagKey,
  children,
  className,
}: FeatureGatedTabTriggerProps) => {
  const hasAccess = useFeatureFlag(flagKey);
  const navigate = useNavigate();
  const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      // In development, allow access to all tabs
      if (process.env.NODE_ENV === "development") {
        return;
      }

      if (!hasAccess) {
        e.preventDefault();
        e.stopPropagation();
        setShowUpgradeDialog(true);
      }
    },
    [hasAccess]
  );

  const handleUpgrade = useCallback(() => {
    setShowUpgradeDialog(false);
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  const handleClose = useCallback(() => {
    setShowUpgradeDialog(false);
  }, []);

  // In development, show all tabs without lock
  const isDevelopment = process.env.NODE_ENV === "development";
  const showLock = !(hasAccess || isDevelopment);

  return (
    <>
      <TabsTrigger
        className={cn(
          "flex items-center gap-2",
          showLock && "cursor-not-allowed opacity-70",
          className
        )}
        disabled={showLock}
        onClick={handleClick}
        value={value}
      >
        {children}
        {showLock ? (
          <HugeiconsIcon
            className="size-3.5 text-muted-foreground"
            icon={LockSquareIcon}
          />
        ) : null}
      </TabsTrigger>

      <AlertDialog onOpenChange={setShowUpgradeDialog} open={showUpgradeDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2">
              <HugeiconsIcon
                className="size-5 text-primary"
                icon={LockSquareIcon}
              />
              Premium Feature
            </AlertDialogTitle>
            <AlertDialogDescription>
              This report tab requires a higher subscription tier. Upgrade your
              plan to access advanced analytics and insights.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleClose}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleUpgrade}>
              <HugeiconsIcon className="size-4" icon={CrownIcon} />
              Upgrade now
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
};

type FeatureGatedTabContentProps = {
  value: string;
  flagKey: FeatureFlagKey;
  children: ReactNode;
  className?: string;
};

/**
 * Tab content that shows an upgrade prompt when the feature is not available.
 */
export const FeatureGatedTabContent = ({
  value,
  flagKey,
  children,
  className,
}: FeatureGatedTabContentProps) => {
  const hasAccess = useFeatureFlag(flagKey);
  const navigate = useNavigate();

  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  // In development, show all content
  const isDevelopment = process.env.NODE_ENV === "development";
  if (hasAccess || isDevelopment) {
    return (
      <TabsContent className={className} value={value}>
        {children}
      </TabsContent>
    );
  }

  // Show upgrade prompt for locked content
  return (
    <TabsContent className={cn("space-y-6", className)} value={value}>
      <div className="flex min-h-[300px] flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-accent">
          <HugeiconsIcon
            className="h-6 w-6 text-accent-foreground"
            icon={LockSquareIcon}
          />
        </div>
        <h3 className="mb-2 font-normal text-lg">Premium Feature</h3>
        <p className="mb-4 max-w-md text-muted-foreground text-sm">
          This report requires a higher subscription tier. Upgrade your plan to
          access advanced analytics and insights.
        </p>
        <Button onClick={handleUpgrade}>
          <HugeiconsIcon className="size-4" icon={CrownIcon} />
          Upgrade Plan
        </Button>
      </div>
    </TabsContent>
  );
};
