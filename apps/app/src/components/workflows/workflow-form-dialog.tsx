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
import { useEffect, useRef, useState } from "react";
import type { Job, PaginatedResponse, Workflow } from "@/hooks/api/types";
import { jobsQueryOptions } from "@/hooks/api/use-jobs";
import { useCreateWorkflow } from "@/hooks/api/use-workflows";
import { WorkflowIcon } from "@/lib/icons";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (workflow: Workflow) => void;
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
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [jobId, setJobId] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const wasOpen = useRef(false);
  const defaultJobId = jobs?.data[0]?.id ?? "";

  useEffect(() => {
    if (open && !wasOpen.current) {
      setName("");
      setDescription("");
      setJobId(defaultJobId);
      setEnabled(true);
      setError(null);
    }
    wasOpen.current = open;
  }, [defaultJobId, open]);

  useEffect(() => {
    if (open && !jobId) {
      setJobId(defaultJobId);
    }
  }, [defaultJobId, jobId, open]);

  async function submit() {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required.");
      return;
    }
    if (!jobId) {
      setError("Select a job for the first workflow step.");
      return;
    }

    setError(null);
    try {
      const workflow = await toast.promise(
        createWorkflow.mutateAsync({
          name: trimmedName,
          description: description.trim(),
          job_id: jobId,
          enabled,
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
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-xl">
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
                onChange={(event) => setName(event.target.value)}
                placeholder="Onboard new account"
                value={name}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="workflow-description">
                Description (optional)
              </FieldLabel>
              <Textarea
                id="workflow-description"
                onChange={(event) => setDescription(event.target.value)}
                placeholder="What does this workflow coordinate?"
                rows={3}
                value={description}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="workflow-job">First job step</FieldLabel>
              <Select
                onValueChange={(value) => setJobId(value ?? "")}
                value={jobId}
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
                  checked={enabled}
                  id="workflow-enabled"
                  onCheckedChange={(checked) => setEnabled(Boolean(checked))}
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
            <Button disabled={createWorkflow.isPending} type="submit">
              {createWorkflow.isPending ? <Spinner /> : null}
              {createWorkflow.isPending ? "Creating..." : "Create workflow"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
