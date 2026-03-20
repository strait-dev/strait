import { Button } from "@strait/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
import { Input } from "@strait/ui/components/input";
import { Label } from "@strait/ui/components/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { useState } from "react";
import { useSetProjectBudget } from "@/hooks/billing/use-project-budget";

type ProjectBudgetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  projectName: string;
  currentBudgetMicro: number;
  currentAction: string;
};

export function ProjectBudgetDialog({
  open,
  onOpenChange,
  projectId,
  projectName,
  currentBudgetMicro,
  currentAction,
}: ProjectBudgetDialogProps) {
  const initialBudget =
    currentBudgetMicro > 0 ? String(currentBudgetMicro / 1_000_000) : "";
  const [budgetUsd, setBudgetUsd] = useState(initialBudget);
  const [action, setAction] = useState(currentAction || "notify");
  const mutation = useSetProjectBudget();

  const handleSave = () => {
    if (!budgetUsd) {
      mutation.mutate(
        { projectId, budgetMicrousd: -1, action },
        { onSuccess: () => onOpenChange(false) }
      );
      return;
    }
    const parsed = Number.parseFloat(budgetUsd);
    if (Number.isNaN(parsed) || !Number.isFinite(parsed) || parsed < 0) {
      return;
    }
    const budgetMicro = Math.round(parsed * 1_000_000);
    mutation.mutate(
      { projectId, budgetMicrousd: budgetMicro, action },
      { onSuccess: () => onOpenChange(false) }
    );
  };

  const handleRemove = () => {
    mutation.mutate(
      { projectId, budgetMicrousd: -1, action: "notify" },
      { onSuccess: () => onOpenChange(false) }
    );
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Set Budget for {projectName}</DialogTitle>
          <DialogDescription>
            Set a monthly spending budget for this project. You can choose to
            reject new runs or just receive a notification when the budget is
            reached.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="budget">Monthly Budget (USD)</Label>
            <Input
              id="budget"
              min="0"
              onChange={(e) => setBudgetUsd(e.target.value)}
              placeholder="e.g. 100"
              step="1"
              type="number"
              value={budgetUsd}
            />
            <p className="text-muted-foreground text-xs">
              Leave empty to remove the budget cap.
            </p>
          </div>

          <div className="space-y-2">
            <Label>When budget is reached</Label>
            <Select
              onValueChange={(v) => {
                if (v) {
                  setAction(v);
                }
              }}
              value={action}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="notify">Notify only (soft cap)</SelectItem>
                <SelectItem value="reject">
                  Reject new runs (hard cap)
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          {currentBudgetMicro > 0 && (
            <Button
              disabled={mutation.isPending}
              onClick={handleRemove}
              variant="ghost"
            >
              Remove Budget
            </Button>
          )}
          <DialogClose render={<Button variant="outline" />}>
            Cancel
          </DialogClose>
          <Button disabled={mutation.isPending} onClick={handleSave}>
            {mutation.isPending ? "Saving..." : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
