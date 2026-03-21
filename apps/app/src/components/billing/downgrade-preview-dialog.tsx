import { Badge } from "@strait/ui/components/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { useQuery } from "@tanstack/react-query";
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

const ActionBadge = ({ action }: { action: string }) => {
  if (action === "ok") {
    return <Badge variant="success-light">OK</Badge>;
  }
  if (action === "reduce") {
    return <Badge variant="warning">Reduce</Badge>;
  }
  return <Badge variant="destructive">Remove</Badge>;
};

const DowngradePreviewContent = ({
  isLoading: isPreviewLoading,
  preview,
  hasIssues,
}: {
  isLoading: boolean;
  preview: DowngradePreview | null | undefined;
  hasIssues: boolean | undefined;
}) => {
  if (isPreviewLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <p className="text-muted-foreground text-sm">Loading preview...</p>
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

  return (
    <div className="space-y-4 py-2">
      {hasIssues ? (
        <div className="rounded-md border border-yellow-200 bg-yellow-50 p-3">
          <p className="text-sm text-yellow-800">
            Some resources exceed the limits of the{" "}
            {capitalize(preview.target_plan)} plan. You may need to reduce usage
            before downgrading.
          </p>
        </div>
      ) : null}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Resource</TableHead>
            <TableHead className="text-right">Current</TableHead>
            <TableHead className="text-right">New Limit</TableHead>
            <TableHead className="text-right">Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {preview.impacts.map((impact) => (
            <TableRow key={impact.resource}>
              <TableCell>{capitalize(impact.resource)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {impact.current.toLocaleString()}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {impact.limit.toLocaleString()}
              </TableCell>
              <TableCell className="text-right">
                <ActionBadge action={impact.action} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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

  const hasIssues = preview?.impacts?.some((i) => i.action !== "ok");

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
          hasIssues={hasIssues}
          isLoading={isPreviewLoading}
          preview={preview}
        />

        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Cancel
          </DialogClose>
          <Button
            disabled={isLoading || isPreviewLoading}
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
