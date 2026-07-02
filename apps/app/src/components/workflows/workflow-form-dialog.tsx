import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Checkbox } from "@strait/ui/components/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Spinner } from "@strait/ui/components/spinner";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import type { Job, PaginatedResponse, Workflow } from "@/hooks/api/types";
import { jobsQueryOptions } from "@/hooks/api/use-jobs";
import { useCreateWorkflow } from "@/hooks/api/use-workflows";
import { WorkflowIcon } from "@/lib/icons";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (workflow: Workflow) => void;
};

type WorkflowFormState = {
  description: string;
  enabled: boolean;
  jobId: string;
  name: string;
};

export default function WorkflowFormDialog({
  open,
  onOpenChange,
  onCreated,
}: Props) {
  const createWorkflow = useCreateWorkflow();
  const { data: jobs } = useQuery(jobsQueryOptions({ limit: 100 })) as {
    data: PaginatedResponse<Job> | undefined;
  };
  const defaultJobId = jobs?.data[0]?.id ?? "";
  const initialForm = useMemo<WorkflowFormState>(
    () => ({
      description: "",
      enabled: true,
      jobId: defaultJobId,
      name: "",
    }),
    [defaultJobId]
  );
  const [formUpdates, setFormUpdates] = useState<Partial<WorkflowFormState>>(
    {}
  );
  const [error, setError] = useState<string | null>(null);
  const form = useMemo(
    () => ({ ...initialForm, ...formUpdates }),
    [formUpdates, initialForm]
  );

  function update<K extends keyof WorkflowFormState>(
    key: K,
    value: WorkflowFormState[K]
  ) {
    setFormUpdates((current) => ({ ...current, [key]: value }));
  }

  function handleOpenChange(nextOpen: boolean) {
    if (nextOpen) {
      setFormUpdates({});
      setError(null);
    }
    onOpenChange(nextOpen);
  }

  async function submit() {
    const trimmedName = form.name.trim();
    if (!trimmedName) {
      setError("Name is required.");
      return;
    }
    if (!form.jobId) {
      setError("Select a job for the first workflow step.");
      return;
    }

    setError(null);
    try {
      const workflow = await toast.promise(
        createWorkflow.mutateAsync({
          name: trimmedName,
          description: form.description.trim(),
          job_id: form.jobId,
          enabled: form.enabled,
        }),
        {
          loading: "Creating workflow...",
          success: "Workflow created.",
          error: "Failed to create workflow.",
        }
      );
      onCreated?.(workflow);
      onOpenChange(false);
    } catch {
      // toast handles the visible error.
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-xl">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <HugeiconsIcon className="size-4" icon={WorkflowIcon} />
              Create workflow
            </DialogTitle>
            <DialogDescription>
              Create a simple workflow with one job step. Advanced step editing
              stays in the API and SDK workflow for now.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <Field>
              <FieldLabel htmlFor="workflow-name">Name</FieldLabel>
              <Input
                autoFocus
                id="workflow-name"
                onChange={(event) => update("name", event.target.value)}
                placeholder="Onboard new account"
                value={form.name}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="workflow-description">
                Description (optional)
              </FieldLabel>
              <Textarea
                id="workflow-description"
                onChange={(event) => update("description", event.target.value)}
                placeholder="What does this workflow coordinate?"
                rows={3}
                value={form.description}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="workflow-job">First job step</FieldLabel>
              <Select
                onValueChange={(value) => update("jobId", value ?? "")}
                value={form.jobId}
              >
                <SelectTrigger id="workflow-job">
                  <SelectValue placeholder="Select a job" />
                </SelectTrigger>
                <SelectContent>
                  {(jobs?.data ?? []).map((job) => (
                    <SelectItem key={job.id} value={job.id}>
                      {job.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <label
                className="flex items-center gap-2 text-sm"
                htmlFor="workflow-enabled"
              >
                <Checkbox
                  checked={form.enabled}
                  id="workflow-enabled"
                  onCheckedChange={(checked) =>
                    update("enabled", Boolean(checked))
                  }
                />
                Enabled
              </label>
            </Field>

            {error ? <FieldError>{error}</FieldError> : null}
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Cancel
            </DialogClose>
            <Button
              disabled={createWorkflow.isPending}
              onClick={() => {
                submit().catch(() => undefined);
              }}
              type="button"
            >
              {createWorkflow.isPending ? <Spinner /> : null}
              {createWorkflow.isPending ? "Creating..." : "Create workflow"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
