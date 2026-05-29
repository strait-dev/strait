import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { KeyIcon } from "@/lib/icons";
import {
  canUseFeature,
  isRoadmapFeature,
  type PlanFeature,
} from "@/lib/plan-tiers";

const FEATURE_LABELS: Record<PlanFeature, { name: string; minPlan: string }> = {
  http_mode: { name: "HTTP Execution Mode", minPlan: "Pro" },
  approval_gates: { name: "Approval Gates", minPlan: "Pro" },
  sub_workflows: { name: "Sub-Workflows", minPlan: "Pro" },
  job_chaining: { name: "Job Chaining", minPlan: "Pro" },
  compensating_txns: { name: "Compensating Transactions", minPlan: "Pro" },
  canary_deployments: { name: "Canary Deployments", minPlan: "Scale" },
  audit_logs: { name: "Audit Logs", minPlan: "Scale" },
  sso: { name: "SSO / SAML", minPlan: "Enterprise" },
  sla: { name: "SLA", minPlan: "Enterprise" },
  dedicated_compute: { name: "Dedicated Compute", minPlan: "Enterprise" },
  static_ips: { name: "Static IPs", minPlan: "Enterprise" },
  vpc_peering: { name: "VPC Peering", minPlan: "Enterprise" },
  scim: { name: "SCIM Directory Sync", minPlan: "Enterprise" },
  data_residency: { name: "Data Residency", minPlan: "Enterprise" },
  custom_rbac: { name: "Custom RBAC", minPlan: "Enterprise" },
  ip_allowlisting: { name: "IP Allowlisting", minPlan: "Enterprise" },
  session_management: { name: "Session Management", minPlan: "Enterprise" },
  secret_rotation: { name: "Secret Rotation", minPlan: "Enterprise" },
  siem_export: { name: "SIEM Export", minPlan: "Enterprise" },
};

type FeatureLockProps = {
  feature: PlanFeature;
  children: ReactNode;
};

/**
 * Wraps content that requires a specific plan feature.
 * Renders children normally when the feature is available.
 * Shows an upgrade prompt overlay when the feature is locked.
 */
const FeatureLock = ({ feature, children }: FeatureLockProps) => {
  const currentPlan = useCurrentPlan();
  const navigate = useNavigate();

  if (canUseFeature(currentPlan, feature)) {
    return <>{children}</>;
  }

  const label = FEATURE_LABELS[feature];
  const roadmap = isRoadmapFeature(feature);

  return (
    <div className="relative">
      <div className="pointer-events-none select-none opacity-30">
        {children}
      </div>
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="flex flex-col items-center gap-3 rounded border border-border bg-card/95 px-6 py-5 shadow-sm backdrop-blur-sm">
          <HugeiconsIcon
            className="size-5 text-muted-foreground"
            icon={KeyIcon}
          />
          <div className="text-center">
            <p className="font-medium text-foreground text-sm">{label.name}</p>
            <p className="text-muted-foreground text-xs">
              {roadmap ? (
                <>
                  Roadmap / contact sales{" "}
                  <Badge className="text-xs" variant="secondary">
                    Launch hidden
                  </Badge>
                </>
              ) : (
                <>
                  Requires the{" "}
                  <Badge className="text-xs" variant="secondary">
                    {label.minPlan}
                  </Badge>{" "}
                  plan or higher
                </>
              )}
            </p>
          </div>
          {roadmap ? (
            <Button
              onClick={() => {
                window.location.assign("/contact");
              }}
              variant="default"
            >
              Contact sales
            </Button>
          ) : (
            <Button
              onClick={() => navigate({ to: "/app/upgrade" })}
              variant="default"
            >
              Upgrade
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};

export default FeatureLock;
