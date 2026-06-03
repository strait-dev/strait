import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Progress } from "@strait/ui/components/progress";
import { ENTERPRISE_TIERS, type EnterpriseTier } from "@/lib/billing-constants";
import { CheckCircleIcon, CheckIcon } from "@/lib/icons";

type EnterpriseOverviewProps = {
  enterpriseTier: string;
  contractEndDate: string;
  computeDiscountPct: number;
  slaUptimePct: number;
  includedCreditMicro: number;
  periodSpendMicro: number;
  creditUsedPercent: number;
};

const formatUsd = (microUsd: number): string =>
  `$${(microUsd / 1_000_000).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

const ENTERPRISE_ACTIVE_FEATURES = [
  "Custom orchestration run allowance",
  "Custom concurrency and step caps",
  "Unlimited history by contract",
  "Consolidated invoicing",
  "Dedicated TAM",
  "Named on-call",
] as const;

const ENTERPRISE_ROADMAP_FEATURES = [
  "SSO/SAML",
  "SCIM directory sync",
  "IP allowlisting",
  "Static egress IPs",
  "VPC peering",
  "Data residency",
  "Single-tenant orchestration",
  "BYO-cloud",
] as const;

export const EnterpriseOverview = ({
  enterpriseTier,
  contractEndDate,
  computeDiscountPct,
  slaUptimePct,
  periodSpendMicro,
  creditUsedPercent,
}: EnterpriseOverviewProps) => {
  const tierConfig = ENTERPRISE_TIERS[enterpriseTier as EnterpriseTier] ?? null;
  const tierName = tierConfig?.name ?? "Enterprise";

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Plan</CardDescription>
            <CardTitle className="text-lg">{tierName}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-xs">
              Contract ends {contractEndDate}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Run allowance</CardDescription>
            <CardTitle className="text-lg">Contracted</CardTitle>
          </CardHeader>
          <CardContent>
            <Progress
              className="h-2"
              value={Math.min(creditUsedPercent, 100)}
            />
            <p className="mt-1 text-muted-foreground text-xs">
              {formatUsd(periodSpendMicro)} used ({creditUsedPercent.toFixed(1)}
              %)
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Compute discount</CardDescription>
            <CardTitle className="text-lg">{computeDiscountPct}% off</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-xs">
              Applied to metered overage
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription>SLA</CardDescription>
            <CardTitle className="text-lg">{slaUptimePct}% uptime</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-xs">
              With automatic service credits
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <HugeiconsIcon className="size-4" icon={CheckCircleIcon} />
            Support
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 sm:grid-cols-3">
            <div>
              <Badge variant="destructive-light">P1</Badge>
              <p className="mt-1 font-medium text-sm">1 hour response</p>
              <p className="text-muted-foreground text-xs">
                24/7 coverage, full outage or data loss
              </p>
            </div>
            <div>
              <Badge variant="warning-light">P2</Badge>
              <p className="mt-1 font-medium text-sm">4 hour response</p>
              <p className="text-muted-foreground text-xs">
                Business hours, significant degradation
              </p>
            </div>
            <div>
              <Badge variant="info-light">P3</Badge>
              <p className="mt-1 font-medium text-sm">24 hour response</p>
              <p className="text-muted-foreground text-xs">
                Business hours, minor issues
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <HugeiconsIcon className="size-4" icon={CheckCircleIcon} />
            Enterprise launch features
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {ENTERPRISE_ACTIVE_FEATURES.map((feature) => (
              <div className="flex items-center gap-2" key={feature}>
                <HugeiconsIcon
                  className="size-4 text-primary"
                  icon={CheckIcon}
                />
                <span className="text-sm">{feature}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            Enterprise roadmap
          </CardTitle>
          <CardDescription>
            Contact sales for roadmap timing and contract-specific commitments.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {ENTERPRISE_ROADMAP_FEATURES.map((feature) => (
              <div className="flex items-center gap-2" key={feature}>
                <Badge variant="outline">Roadmap</Badge>
                <span className="text-sm">{feature}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
};
