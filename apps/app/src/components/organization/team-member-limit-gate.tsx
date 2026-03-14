import { Crown03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import type React from "react";
import { AddMoreGate } from "../feature-gates/limit-gate";

type TeamMemberLimitGateProps = {
  currentCount: number;
  children: React.ReactNode;
  onUpgradeClick?: () => void;
};

/**
 * Gate component that enforces team member limits based on subscription plan
 */
export const TeamMemberLimitGate = ({
  currentCount,
  children,
  onUpgradeClick,
}: TeamMemberLimitGateProps) => {
  return (
    <AddMoreGate
      additionalCount={1}
      currentCount={currentCount}
      feature="teamMembersPerStore"
      upgradePrompt={({
        limit: availableLimit,
        currentCount: existingCount,
        nextPlan,
      }) => (
        <Alert className="border-accent bg-accent/50">
          <HugeiconsIcon
            className="h-4 w-4 text-accent-foreground"
            icon={Crown03Icon}
          />
          <AlertDescription className="text-accent-foreground">
            <div className="space-y-2">
              <p className="font-medium">Team member limit reached</p>
              <p className="text-sm">
                Your current plan allows {availableLimit} team member
                {availableLimit === 1 ? "" : "s"}, and you already have{" "}
                {existingCount}.
                {nextPlan ? (
                  <span>
                    {" "}
                    Upgrade to {nextPlan.name} to add more team members.
                  </span>
                ) : null}
              </p>
              {onUpgradeClick ? (
                <button
                  className="inline-flex items-center gap-1 rounded bg-accent-foreground px-3 py-1 font-medium text-accent text-sm hover:bg-accent-foreground/80"
                  onClick={onUpgradeClick}
                  type="button"
                >
                  <HugeiconsIcon className="size-3" icon={Crown03Icon} />
                  Upgrade Plan
                </button>
              ) : null}
            </div>
          </AlertDescription>
        </Alert>
      )}
    >
      {children}
    </AddMoreGate>
  );
};
