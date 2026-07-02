import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Input } from "@strait/ui/components/input";
import { NoticeBanner } from "@strait/ui/components/notice-banner";
import { Progress } from "@strait/ui/components/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Switch } from "@strait/ui/components/switch";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  spendingLimitQueryOptions,
  useUpdateSpendingLimit,
} from "@/hooks/billing/use-spending-limit";
import { capitalize } from "@/lib/format";

const MICRO_USD = 1_000_000;

const PRESET_AMOUNTS = [0, 25, 50, 100, 250, 500];

const SpendingLimitsTab = () => {
  const { data: spending } = useQuery(spendingLimitQueryOptions());
  const updateLimit = useUpdateSpendingLimit();

  const [selectedAmount, setSelectedAmount] = useState<number | null>(null);
  const [customAmount, setCustomAmount] = useState("");
  const [action, setAction] = useState("reject");
  const [overageEnabled, setOverageEnabled] = useState<boolean | null>(null);

  if (!spending) {
    return (
      <Card>
        <CardContent className="flex h-48 items-center justify-center">
          <Empty border={false}>
            <EmptyHeader>
              <EmptyTitle>Spending limit data unavailable</EmptyTitle>
              <EmptyDescription>
                Billing limits will appear here once usage data is available.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </CardContent>
      </Card>
    );
  }

  const planName = capitalize(spending.plan_tier);
  const percent =
    spending.spending_limit_usd > 0
      ? (spending.current_spend_usd / spending.spending_limit_usd) * 100
      : 0;

  const getResolvedAmount = (): number | null => {
    if (selectedAmount !== null) {
      return selectedAmount;
    }
    if (customAmount) {
      return Number(customAmount);
    }
    return null;
  };

  const resolvedAmount = getResolvedAmount();
  const effectiveOverageEnabled = overageEnabled ?? spending.overage_enabled;

  const handlePresetClick = (amount: number) => {
    setSelectedAmount(amount);
    setCustomAmount("");
  };

  const handleCustomChange = (value: string) => {
    setCustomAmount(value);
    setSelectedAmount(null);
  };

  const handleSave = () => {
    if (resolvedAmount === null || Number.isNaN(resolvedAmount)) {
      return;
    }

    updateLimit.mutate(
      {
        limitMicrousd: resolvedAmount * MICRO_USD,
        action,
        overageEnabled: effectiveOverageEnabled,
      },
      {
        onSuccess: () => {
          toast.success("Spending limit updated");
          setSelectedAmount(null);
          setCustomAmount("");
          setOverageEnabled(null);
        },
        onError: () => {
          toast.error("Failed to update spending limit");
        },
      }
    );
  };

  return (
    <div className="space-y-6">
      {spending.is_hard_capped && (
        <NoticeBanner title="Hard spending cap enabled" variant="warning">
          Services will be paused when the limit is reached.
        </NoticeBanner>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 font-medium text-sm">
            Spending limit
            <Badge variant="default">{planName}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div>
            <div className="flex items-baseline justify-between">
              <p className="text-muted-foreground text-sm">Current spend</p>
              <p className="font-medium text-foreground text-lg tabular-nums">
                ${spending.current_spend_usd.toFixed(2)}
                <span className="text-muted-foreground text-sm">
                  {" "}
                  / ${spending.spending_limit_usd.toFixed(2)}
                </span>
              </p>
            </div>
            <Progress
              className="mt-2"
              size="lg"
              value={Math.min(percent, 100)}
              variant={percent >= 90 ? "warning" : "default"}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-muted-foreground text-xs">
                Included allowance
              </p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                Plan run allowance
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Overage spend</p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                ${spending.overage_spend_usd.toFixed(2)}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between border-border border-t pt-4">
            <div>
              <p className="font-medium text-foreground text-sm">Overage</p>
              <p className="mt-1 text-muted-foreground text-xs">
                Allow runs beyond the included monthly allowance.
              </p>
            </div>
            <Switch
              checked={effectiveOverageEnabled}
              onCheckedChange={setOverageEnabled}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">
            Set spending limit
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Preset buttons */}
          <div>
            <p className="mb-2 text-muted-foreground text-xs">
              Choose a preset
            </p>
            <div className="flex flex-wrap gap-2">
              {PRESET_AMOUNTS.map((amount) => (
                <Button
                  key={amount}
                  onClick={() => handlePresetClick(amount)}
                  variant={selectedAmount === amount ? "default" : "outline"}
                >
                  ${amount}
                </Button>
              ))}
            </div>
          </div>

          {/* Custom input */}
          <div>
            <p className="mb-2 text-muted-foreground text-xs">
              Or enter a custom amount
            </p>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground text-sm">$</span>
              <Input
                className="max-w-[160px]"
                min={0}
                onChange={(e) => handleCustomChange(e.target.value)}
                placeholder="Custom amount"
                type="number"
                value={customAmount}
              />
            </div>
          </div>

          {/* Action toggle */}
          <div>
            <p className="mb-2 text-muted-foreground text-xs">
              When limit is reached
            </p>
            <Select
              onValueChange={(v) => {
                if (v) {
                  setAction(v);
                }
              }}
              value={action}
            >
              <SelectTrigger className="w-[200px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="reject">Reject new runs</SelectItem>
                <SelectItem value="notify">Notify only</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Save button */}
          <Button
            disabled={
              resolvedAmount === null ||
              Number.isNaN(resolvedAmount) ||
              updateLimit.isPending
            }
            onClick={handleSave}
          >
            {updateLimit.isPending ? "Saving..." : "Save spending limit"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
};

export default SpendingLimitsTab;
