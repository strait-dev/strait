import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { toast } from "@strait/ui/components/toast/index";
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

  if (!spending) {
    return (
      <Card>
        <CardContent className="flex h-48 items-center justify-center">
          <p className="text-muted-foreground text-sm">
            Spending limit data unavailable.
          </p>
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
      { limitMicrousd: resolvedAmount * MICRO_USD, action },
      {
        onSuccess: () => {
          toast.success("Spending limit updated");
          setSelectedAmount(null);
          setCustomAmount("");
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
        <Card className="border-yellow-200 dark:border-yellow-800">
          <CardContent className="p-4">
            <p className="text-sm text-yellow-800 dark:text-yellow-200">
              Hard spending cap is enabled. Services will be paused when the
              limit is reached.
            </p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 font-medium text-sm">
            Spending Limit
            <Badge variant="default">{planName}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div>
            <div className="flex items-baseline justify-between">
              <p className="text-muted-foreground text-sm">Current Spend</p>
              <p className="font-medium text-foreground text-lg tabular-nums">
                ${spending.current_spend_usd.toFixed(2)}
                <span className="text-muted-foreground text-sm">
                  {" "}
                  / ${spending.spending_limit_usd.toFixed(2)}
                </span>
              </p>
            </div>
            <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-foreground transition-all"
                style={{ width: `${Math.min(percent, 100)}%` }}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-muted-foreground text-xs">Included Credit</p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                ${spending.included_credit_usd.toFixed(2)}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Overage Spend</p>
              <p className="mt-1 font-medium text-foreground tabular-nums">
                ${spending.overage_spend_usd.toFixed(2)}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">
            Set Spending Limit
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
                  size="sm"
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
            <Select onValueChange={(v) => { if (v) { setAction(v); } }} value={action}>
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
            {updateLimit.isPending ? "Saving..." : "Save Spending Limit"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
};

export default SpendingLimitsTab;
