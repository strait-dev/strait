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
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  projectBudgetQueryOptions,
  useSetProjectBudget,
} from "@/hooks/billing/use-project-budget";

type ProjectBudgetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  projectName: string;
  currentBudgetMicro: number;
  currentAction: string;
};

const ProjectBudgetDialog = ({
  open,
  onOpenChange,
  projectId,
  projectName,
  currentBudgetMicro,
  currentAction,
}: ProjectBudgetDialogProps) => {
  const { data: budgetData } = useQuery({
    ...projectBudgetQueryOptions(projectId),
    enabled: open,
  });
  const liveBudgetMicro =
    budgetData?.monthly_budget_microusd ?? currentBudgetMicro;
  const liveBudgetAction = budgetData?.budget_action ?? currentAction;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Set budget for {projectName}</DialogTitle>
          <DialogDescription>
            Set a monthly spending budget for this project. You can choose to
            reject new runs or just receive a notification when the budget is
            reached.
          </DialogDescription>
        </DialogHeader>

        <BudgetForm
          budgetAction={liveBudgetAction}
          budgetMicro={liveBudgetMicro}
          key={`${liveBudgetMicro}-${liveBudgetAction}`}
          onClose={() => onOpenChange(false)}
          projectId={projectId}
        />
      </DialogContent>
    </Dialog>
  );
};

const BudgetForm = ({
  budgetMicro,
  budgetAction,
  projectId,
  onClose,
}: {
  budgetMicro: number;
  budgetAction: string;
  projectId: string;
  onClose: () => void;
}) => {
  const [budgetUsd, setBudgetUsd] = useState(
    budgetMicro > 0 ? String(budgetMicro / 1_000_000) : ""
  );
  const [action, setAction] = useState(budgetAction || "notify");
  const mutation = useSetProjectBudget();

  const handleSave = () => {
    if (!budgetUsd) {
      mutation.mutate(
        { projectId, budgetMicrousd: -1, action },
        { onSuccess: onClose }
      );
      return;
    }
    const parsed = Number.parseFloat(budgetUsd);
    if (Number.isNaN(parsed) || !Number.isFinite(parsed) || parsed < 0) {
      return;
    }
    const micro = Math.round(parsed * 1_000_000);
    mutation.mutate(
      { projectId, budgetMicrousd: micro, action },
      { onSuccess: onClose }
    );
  };

  const handleRemove = () => {
    mutation.mutate(
      { projectId, budgetMicrousd: -1, action: "notify" },
      { onSuccess: onClose }
    );
  };

  return (
    <>
      <div className="space-y-4 py-4">
        <div className="space-y-2">
          <Label htmlFor="budget">Monthly budget (USD)</Label>
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
              <SelectItem value="reject">Reject new runs (hard cap)</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <DialogFooter>
        {budgetMicro > 0 && (
          <Button
            disabled={mutation.isPending}
            onClick={handleRemove}
            variant="ghost"
          >
            Remove budget
          </Button>
        )}
        <DialogClose render={<Button variant="outline" />}>Cancel</DialogClose>
        <Button disabled={mutation.isPending} onClick={handleSave}>
          {mutation.isPending ? "Saving..." : "Save"}
        </Button>
      </DialogFooter>
    </>
  );
};

export default ProjectBudgetDialog;
