import { Badge } from "@strait/ui/components/badge";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { canUseFeature, type PlanFeature } from "@/lib/plan-tiers";

const MIN_PLAN_LABEL: Record<PlanFeature, string> = {
  http_mode: "Pro",
  approval_gates: "Pro",
  sub_workflows: "Pro",
  job_chaining: "Pro",
  compensating_txns: "Pro",
  canary_deployments: "Scale",
  audit_logs: "Scale",
  sso: "Enterprise",
  sla: "Enterprise",
};

type FeatureBadgeProps = {
  feature: PlanFeature;
};

/**
 * Shows a plan badge (e.g. "Pro", "Scale") next to a feature name
 * when the current plan doesn't include that feature.
 * Renders nothing when the feature is available.
 */
const FeatureBadge = ({ feature }: FeatureBadgeProps) => {
  const currentPlan = useCurrentPlan();

  if (canUseFeature(currentPlan, feature)) {
    return null;
  }

  return (
    <Badge className="ml-1.5 text-[10px]" variant="outline">
      {MIN_PLAN_LABEL[feature]}
    </Badge>
  );
};

export default FeatureBadge;
