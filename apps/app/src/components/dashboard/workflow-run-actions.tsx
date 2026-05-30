import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";
import StatusBadge from "@/components/dashboard/status-badge";
import type {
  WorkflowRun,
  WorkflowRunChainEntry,
  WorkflowRunStatus,
} from "@/hooks/api/types";
import {
  useContinueWorkflowRunAsNew,
  workflowRunChainQueryOptions,
} from "@/hooks/api/use-workflows";
import {
  ChevronRightIcon,
  LinkSquareIcon,
  LoadingIcon,
  MoreVerticalIcon,
} from "@/lib/icons";
import {
  type ContinueVersionStrategy,
  canContinueWorkflowRun,
  DEFAULT_CONTINUE_VERSION_STRATEGY,
  isPartOfChain,
  parseContinueInput,
} from "@/lib/workflow-continue";

type WorkflowRunActionsProps = {
  run: WorkflowRun;
};

/**
 * Per-row actions for a workflow run: continue-as-new (for running or paused
 * runs) and chain navigation (for runs that belong to a continuation lineage).
 * The dialogs are controlled by state and rendered as siblings of the menu so
 * dismissing the menu does not unmount an open dialog.
 */
const WorkflowRunActions = ({ run }: WorkflowRunActionsProps) => {
  const [continueOpen, setContinueOpen] = useState(false);
  const [chainOpen, setChainOpen] = useState(false);

  const canContinue = canContinueWorkflowRun(run.status);
  const inChain = isPartOfChain(run);

  if (!(canContinue || inChain)) {
    return null;
  }

  return (
    <div className="text-right">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button aria-label="Run actions" size="icon" variant="ghost" />
          }
        >
          <HugeiconsIcon className="size-4" icon={MoreVerticalIcon} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {canContinue && (
            <DropdownMenuItem onClick={() => setContinueOpen(true)}>
              <HugeiconsIcon
                className="mr-2 size-3.5"
                icon={ChevronRightIcon}
              />
              Continue as new
            </DropdownMenuItem>
          )}
          {inChain && (
            <DropdownMenuItem onClick={() => setChainOpen(true)}>
              <HugeiconsIcon className="mr-2 size-3.5" icon={LinkSquareIcon} />
              View chain
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {canContinue && (
        <ContinueAsNewDialog
          onOpenChange={setContinueOpen}
          open={continueOpen}
          run={run}
        />
      )}
      {inChain && (
        <WorkflowRunChainDialog
          onOpenChange={setChainOpen}
          open={chainOpen}
          run={run}
        />
      )}
    </div>
  );
};

type DialogProps = {
  run: WorkflowRun;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function ContinueAsNewDialog({ run, open, onOpenChange }: DialogProps) {
  const continueRun = useContinueWorkflowRunAsNew();
  const [inputText, setInputText] = useState("");
  const [strategy, setStrategy] = useState<ContinueVersionStrategy>(
    DEFAULT_CONTINUE_VERSION_STRATEGY
  );
  const [inputError, setInputError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const parsed = parseContinueInput(inputText);
    if (!parsed.ok) {
      setInputError(parsed.error);
      return;
    }
    setInputError(null);
    try {
      const successor = await continueRun.mutateAsync({
        workflowRunId: run.id,
        workflowId: run.workflow_id,
        input: parsed.value,
        versionStrategy: strategy,
      });
      toast.success(`Started successor run ${successor.id.slice(0, 8)}`);
      setInputText("");
      onOpenChange(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to continue run."
      );
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Continue as new</DialogTitle>
            <DialogDescription>
              Complete run {run.id.slice(0, 8)} and start a fresh successor run
              of the same workflow with empty step history. The successor links
              back to this run.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-4">
            <Field>
              <FieldLabel htmlFor="continue-input">
                Carry-over input (JSON, optional)
              </FieldLabel>
              <Textarea
                aria-invalid={Boolean(inputError)}
                className="font-mono text-xs"
                id="continue-input"
                onChange={(event) => {
                  setInputText(event.target.value);
                  if (inputError) {
                    setInputError(null);
                  }
                }}
                placeholder={'{\n  "cursor": 1\n}'}
                rows={6}
                value={inputText}
              />
              {inputError && <FieldError>{inputError}</FieldError>}
            </Field>

            <Field>
              <FieldLabel>Version strategy</FieldLabel>
              <Select
                onValueChange={(value) =>
                  setStrategy(value as ContinueVersionStrategy)
                }
                value={strategy}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="repin">
                    Repin — reuse this run's version
                  </SelectItem>
                  <SelectItem value="latest">
                    Latest — adopt the newest published version
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>

          <DialogFooter>
            <Button
              onClick={() => onOpenChange(false)}
              type="button"
              variant="outline"
            >
              Cancel
            </Button>
            <Button disabled={continueRun.isPending} type="submit">
              {continueRun.isPending && (
                <HugeiconsIcon
                  className="mr-1.5 size-4 animate-spin"
                  icon={LoadingIcon}
                />
              )}
              Continue as new
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function WorkflowRunChainDialog({ run, open, onOpenChange }: DialogProps) {
  const { data, isLoading, isError } = useQuery({
    ...workflowRunChainQueryOptions(run.id),
    enabled: open,
  });
  const entries = data?.data ?? [];

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Continuation chain</DialogTitle>
          <DialogDescription>
            Runs in this continue-as-new lineage, oldest first.
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[420px] overflow-auto py-2">
          {isError && (
            <p className="py-6 text-center text-muted-foreground text-sm">
              The chain is unavailable right now.
            </p>
          )}
          {!isError && isLoading && (
            <p className="py-6 text-center text-muted-foreground text-sm">
              Loading chain…
            </p>
          )}
          {!(isError || isLoading) && entries.length === 0 && (
            <p className="py-6 text-center text-muted-foreground text-sm">
              No chain entries found.
            </p>
          )}
          {!(isError || isLoading) && entries.length > 0 && (
            <ol className="flex flex-col gap-1.5">
              {entries.map((entry) => (
                <ChainEntryRow
                  current={entry.id === run.id}
                  entry={entry}
                  key={entry.id}
                  onNavigate={() => onOpenChange(false)}
                />
              ))}
            </ol>
          )}
        </div>

        <DialogFooter>
          <Button
            onClick={() => onOpenChange(false)}
            type="button"
            variant="outline"
          >
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ChainEntryRow({
  entry,
  current,
  onNavigate,
}: {
  entry: WorkflowRunChainEntry;
  current: boolean;
  onNavigate: () => void;
}) {
  const timestamp = entry.finished_at ?? entry.started_at ?? entry.created_at;
  return (
    <li>
      <Link
        className={
          current
            ? "flex items-center gap-3 rounded-md border border-primary/40 bg-accent px-3 py-2"
            : "flex items-center gap-3 rounded-md border border-transparent px-3 py-2 hover:bg-accent"
        }
        onClick={onNavigate}
        params={{ id: entry.id }}
        to="/app/workflow-runs/$id"
      >
        <span className="w-8 shrink-0 font-mono text-muted-foreground text-xs">
          #{entry.lineage_depth}
        </span>
        <StatusBadge
          showDot
          size="xs"
          status={entry.status as WorkflowRunStatus}
        />
        <span className="font-mono text-xs">{entry.id.slice(0, 8)}</span>
        <Badge className="capitalize" size="xs" variant="outline">
          {entry.triggered_by}
        </Badge>
        {current && (
          <Badge size="xs" variant="secondary">
            Current
          </Badge>
        )}
        <span className="ml-auto text-muted-foreground text-xs">
          {formatDistanceToNow(new Date(timestamp), { addSuffix: true })}
        </span>
      </Link>
    </li>
  );
}

export default WorkflowRunActions;
