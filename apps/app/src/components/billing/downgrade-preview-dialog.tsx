import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  CardCheckboxGroup,
  CardCheckboxItem,
} from "@strait/ui/components/card-checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
import { Spinner } from "@strait/ui/components/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  type DowngradePreview,
  downgradePreviewQueryOptions,
} from "@/hooks/billing/use-downgrade-preview";
import { capitalize } from "@/lib/format";

type DowngradePreviewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetTier: string;
  onConfirm: () => void;
  isLoading?: boolean;
};

const actionBadge = (action: string) => {
  if (action === "ok") {
    return { label: "OK", variant: "success-light" as const };
  }
  if (action === "reduce") {
    return { label: "Reduce", variant: "warning" as const };
  }
  return { label: "Remove", variant: "destructive" as const };
};

const DowngradePreviewContent = ({
  isLoading: isPreviewLoading,
  preview,
  hasIssues,
  checkedActions,
  onToggleAction,
}: {
  isLoading: boolean;
  preview: DowngradePreview | null | undefined;
  hasIssues: boolean | undefined;
  checkedActions: Record<string, boolean>;
  onToggleAction: (resource: string, checked: boolean) => void;
}) => {
  if (isPreviewLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  if (!preview) {
    return (
      <div className="flex h-32 items-center justify-center">
        <p className="text-muted-foreground text-sm">
          Unable to load downgrade preview.
        </p>
      </div>
    );
  }

  const manualActions = (preview.manual_actions ?? []).filter(
    (i) => i.action !== "ok"
  );

  return (
    <div className="space-y-4 py-2">
      {preview.effective_date ? (
        <Alert>
          <AlertDescription>
            Effective date:{" "}
            <span className="font-medium text-foreground">
              {new Date(preview.effective_date).toLocaleDateString("en-US", {
                year: "numeric",
                month: "long",
                day: "numeric",
              })}
            </span>
            <span className="mt-1 block text-xs">
              Your current plan will remain active until this date.
            </span>
          </AlertDescription>
        </Alert>
      ) : null}

      {hasIssues ? (
        <Alert variant="warning">
          <AlertDescription>
            Some resources exceed the limits of the{" "}
            {capitalize(preview.target_plan)} plan. You may need to reduce usage
            before downgrading.
          </AlertDescription>
        </Alert>
      ) : null}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Resource</TableHead>
            <TableHead className="text-right">Current</TableHead>
            <TableHead className="text-right">New limit</TableHead>
            <TableHead className="text-right">Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {preview.impacts.map((impact) => {
            const badge = actionBadge(impact.action);
            return (
              <TableRow key={impact.resource}>
                <TableCell>{capitalize(impact.resource)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {impact.current.toLocaleString()}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {impact.limit.toLocaleString()}
                </TableCell>
                <TableCell className="text-right">
                  <Badge variant={badge.variant}>{badge.label}</Badge>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      {manualActions.length > 0 ? (
        <div className="space-y-3">
          <p className="font-medium text-sm">
            Required actions before downgrade
          </p>
          <CardCheckboxGroup>
            {manualActions.map((action) => {
              const checkboxId = `downgrade-action-${action.resource}`;
              return (
                <CardCheckboxItem
                  checked={!!checkedActions[action.resource]}
                  description={`Current: ${action.current.toLocaleString()} / New limit: ${action.limit.toLocaleString()}`}
                  id={checkboxId}
                  key={action.resource}
                  label={`${action.action === "reduce" ? "Reduce" : "Remove"} ${action.resource.toLowerCase()}`}
                  layout="start"
                  onCheckedChange={(checked) =>
                    onToggleAction(action.resource, !!checked)
                  }
                />
              );
            })}
          </CardCheckboxGroup>
        </div>
      ) : null}
    </div>
  );
};

const DowngradePreviewDialog = ({
  open,
  onOpenChange,
  targetTier,
  onConfirm,
  isLoading,
}: DowngradePreviewDialogProps) => {
  const { data: preview, isLoading: isPreviewLoading } = useQuery({
    ...downgradePreviewQueryOptions(targetTier),
    enabled: open && !!targetTier,
  });

  const [checkedActions, setCheckedActions] = useState<Record<string, boolean>>(
    {}
  );

  const hasIssues = preview?.impacts?.some((i) => i.action !== "ok");

  const manualActions = (preview?.manual_actions ?? []).filter(
    (i) => i.action !== "ok"
  );
  const allChecked =
    manualActions.length === 0 ||
    manualActions.every((a) => checkedActions[a.resource]);

  const handleToggleAction = (resource: string, checked: boolean) => {
    setCheckedActions((prev) => ({ ...prev, [resource]: checked }));
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Downgrade to {capitalize(targetTier)}</DialogTitle>
          <DialogDescription>
            Review the impact of downgrading your plan before proceeding.
          </DialogDescription>
        </DialogHeader>

        <DowngradePreviewContent
          checkedActions={checkedActions}
          hasIssues={hasIssues}
          isLoading={isPreviewLoading}
          onToggleAction={handleToggleAction}
          preview={preview}
        />

        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Cancel
          </DialogClose>
          <Button
            disabled={isLoading || isPreviewLoading || !allChecked}
            onClick={onConfirm}
            variant="default"
          >
            {isLoading ? "Processing..." : "Proceed with Downgrade"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default DowngradePreviewDialog;
