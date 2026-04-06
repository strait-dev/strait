import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { agentUsageQueryOptions } from "@/hooks/api/use-agent-billing";

type AgentUpgradeNudgeBannerProps = {
  orgId: string;
};

const DISMISS_KEY_PREFIX = "strait:agent_upgrade_dismissed:";

/**
 * Shows an upgrade recommendation banner when the backend detects
 * that the user would benefit from upgrading their agent plan.
 * Dismissible per billing period via localStorage.
 */
function AgentUpgradeNudgeBanner({ orgId }: AgentUpgradeNudgeBannerProps) {
  const { data } = useQuery(agentUsageQueryOptions(orgId));
  const [dismissed, setDismissed] = useState(
    () => localStorage.getItem(`${DISMISS_KEY_PREFIX}${orgId}`) === "true"
  );

  const dismiss = useCallback(() => {
    setDismissed(true);
    localStorage.setItem(`${DISMISS_KEY_PREFIX}${orgId}`, "true");
  }, [orgId]);

  if (!(data?.upgrade_recommended && data.upgrade_reason) || dismissed) {
    return null;
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-accent/5 p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1">
          <p className="font-medium text-sm">{data.upgrade_reason}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <a href="/app/upgrade">
            <Button size="sm" variant="default">
              Upgrade
            </Button>
          </a>
          <Button onClick={dismiss} size="sm" variant="ghost">
            Dismiss
          </Button>
        </div>
      </div>
    </div>
  );
}

export default AgentUpgradeNudgeBanner;
