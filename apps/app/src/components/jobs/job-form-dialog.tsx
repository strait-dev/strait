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
import { useMemo, useState } from "react";
import type { Job } from "@/hooks/api/types";
import {
  type CreateJobInput,
  type JobMutationInput,
  type UpdateJobInput,
  useCreateJob,
  useUpdateJob,
} from "@/hooks/api/use-jobs";
import { BriefcaseIcon, ClockIcon } from "@/lib/icons";

type JobFormKind = "job" | "schedule";

type JobFormState = {
  name: string;
  description: string;
  endpoint_url: string;
  cron: string;
  max_attempts: string;
  timeout_secs: string;
  retry_strategy: NonNullable<JobMutationInput["retry_strategy"]>;
  execution_mode: NonNullable<JobMutationInput["execution_mode"]>;
  queue_name: string;
  enabled: boolean;
};

type Props = {
  kind: JobFormKind;
  job?: Job | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved?: (job: Job) => void;
};

const retryStrategies: JobFormState["retry_strategy"][] = [
  "exponential",
  "linear",
  "fixed",
  "custom",
];

const executionModes: JobFormState["execution_mode"][] = ["http", "worker"];

function initialState(kind: JobFormKind, job?: Job | null): JobFormState {
  return {
    name: job?.name ?? "",
    description: job?.description ?? "",
    endpoint_url: job?.endpoint_url ?? "",
    cron: job?.cron ?? (kind === "schedule" ? "*/5 * * * *" : ""),
    max_attempts: String(job?.max_attempts ?? 3),
    timeout_secs: String(job?.timeout_secs ?? 300),
    retry_strategy:
      (job?.retry_strategy as JobFormState["retry_strategy"]) ?? "exponential",
    execution_mode:
      (job?.execution_mode as JobFormState["execution_mode"]) ?? "http",
    queue_name: job?.queue ?? "",
    enabled: job?.enabled ?? true,
  };
}

export default function JobFormDialog({
  kind,
  job,
  open,
  onOpenChange,
  onSaved,
}: Props) {
  const createJob = useCreateJob();
  const updateJob = useUpdateJob();
  const initialForm = useMemo(() => initialState(kind, job), [job, kind]);
  const [formUpdates, setFormUpdates] = useState<Partial<JobFormState>>({});
  const [error, setError] = useState<string | null>(null);
  const form = useMemo(
    () => ({ ...initialForm, ...formUpdates }),
    [formUpdates, initialForm]
  );
  const isEditing = Boolean(job?.id);
  const isPending = createJob.isPending || updateJob.isPending;
  const labels = useMemo(
    () =>
      kind === "schedule"
        ? {
            title: isEditing ? "Edit schedule" : "Create schedule",
            description:
              "Configure a cron-enabled job that Strait can run on schedule or on demand.",
            submit: isEditing ? "Save schedule" : "Create schedule",
          }
        : {
            title: isEditing ? "Edit job" : "Create job",
            description:
              "Configure a job that Strait can trigger over HTTP or dispatch to a connected worker.",
            submit: isEditing ? "Save job" : "Create job",
          },
    [isEditing, kind]
  );

  function update<K extends keyof JobFormState>(
    key: K,
    value: JobFormState[K]
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
    const payload = buildPayload(kind, form);
    if ("error" in payload) {
      setError(payload.error);
      return;
    }

    setError(null);
    try {
      const saved = isEditing
        ? await toast.promise(
            updateJob.mutateAsync({
              id: job?.id ?? "",
              ...payload.data,
            } satisfies UpdateJobInput),
            {
              loading: "Saving changes...",
              success: "Changes saved.",
              error: "Failed to save changes.",
            }
          )
        : await toast.promise(
            createJob.mutateAsync(payload.data as CreateJobInput),
            {
              loading: `Creating ${kind}...`,
              success: `${kind === "schedule" ? "Schedule" : "Job"} created.`,
              error: `Failed to create ${kind}.`,
            }
          );
      onSaved?.(saved);
      onOpenChange(false);
    } catch {
      // toast handles the visible error.
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-2xl">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <HugeiconsIcon
                className="size-4"
                icon={kind === "schedule" ? ClockIcon : BriefcaseIcon}
              />
              {labels.title}
            </DialogTitle>
            <DialogDescription>{labels.description}</DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4 sm:grid-cols-2">
            <Field className="sm:col-span-2">
              <FieldLabel htmlFor="job-name">Name</FieldLabel>
              <Input
                autoFocus
                id="job-name"
                onChange={(event) => update("name", event.target.value)}
                placeholder={
                  kind === "schedule" ? "Nightly cleanup" : "Send welcome email"
                }
                value={form.name}
              />
            </Field>

            <Field className="sm:col-span-2">
              <FieldLabel htmlFor="job-description">
                Description (optional)
              </FieldLabel>
              <Textarea
                id="job-description"
                onChange={(event) => update("description", event.target.value)}
                placeholder="What does this job do?"
                rows={3}
                value={form.description}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="job-execution-mode">
                Execution mode
              </FieldLabel>
              <Select
                onValueChange={(value) =>
                  update(
                    "execution_mode",
                    value as JobFormState["execution_mode"]
                  )
                }
                value={form.execution_mode}
              >
                <SelectTrigger id="job-execution-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {executionModes.map((mode) => (
                    <SelectItem key={mode} value={mode}>
                      {mode === "http" ? "HTTP endpoint" : "gRPC worker"}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor="job-queue-name">
                Queue name (optional)
              </FieldLabel>
              <Input
                id="job-queue-name"
                onChange={(event) => update("queue_name", event.target.value)}
                placeholder="default"
                value={form.queue_name}
              />
            </Field>

            {form.execution_mode === "http" ? (
              <Field className="sm:col-span-2">
                <FieldLabel htmlFor="job-endpoint-url">Endpoint URL</FieldLabel>
                <Input
                  id="job-endpoint-url"
                  onChange={(event) =>
                    update("endpoint_url", event.target.value)
                  }
                  placeholder="https://api.example.com/jobs/send-welcome-email"
                  value={form.endpoint_url}
                />
              </Field>
            ) : null}

            <Field>
              <FieldLabel htmlFor="job-max-attempts">Max attempts</FieldLabel>
              <Input
                id="job-max-attempts"
                min={1}
                onChange={(event) => update("max_attempts", event.target.value)}
                type="number"
                value={form.max_attempts}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="job-timeout-secs">
                Timeout seconds
              </FieldLabel>
              <Input
                id="job-timeout-secs"
                min={1}
                onChange={(event) => update("timeout_secs", event.target.value)}
                type="number"
                value={form.timeout_secs}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="job-retry-strategy">
                Retry strategy
              </FieldLabel>
              <Select
                onValueChange={(value) =>
                  update(
                    "retry_strategy",
                    value as JobFormState["retry_strategy"]
                  )
                }
                value={form.retry_strategy}
              >
                <SelectTrigger id="job-retry-strategy">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {retryStrategies.map((strategy) => (
                    <SelectItem key={strategy} value={strategy}>
                      {strategy}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor="job-cron">
                Cron {kind === "schedule" ? "" : "(optional)"}
              </FieldLabel>
              <Input
                id="job-cron"
                onChange={(event) => update("cron", event.target.value)}
                placeholder="*/5 * * * *"
                value={form.cron}
              />
            </Field>

            <Field className="sm:col-span-2">
              <label
                className="flex items-center gap-2 text-sm"
                htmlFor="job-enabled"
              >
                <Checkbox
                  checked={form.enabled}
                  id="job-enabled"
                  onCheckedChange={(checked) =>
                    update("enabled", Boolean(checked))
                  }
                />
                Enabled
              </label>
            </Field>

            {error ? (
              <FieldError className="sm:col-span-2">{error}</FieldError>
            ) : null}
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Cancel
            </DialogClose>
            <Button disabled={isPending} type="submit">
              {isPending ? <Spinner /> : null}
              {isPending ? "Saving..." : labels.submit}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function buildPayload(
  kind: JobFormKind,
  form: JobFormState
): { data: CreateJobInput } | { error: string } {
  const name = form.name.trim();
  const executionMode = form.execution_mode;
  const endpointURL = form.endpoint_url.trim();
  const cron = form.cron.trim();
  const maxAttempts = Number(form.max_attempts);
  const timeoutSecs = Number(form.timeout_secs);

  if (!name) {
    return { error: "Name is required." };
  }
  if (executionMode === "http" && !endpointURL) {
    return { error: "Endpoint URL is required for HTTP jobs." };
  }
  if (kind === "schedule" && !cron) {
    return { error: "Cron is required for schedules." };
  }
  if (!Number.isInteger(maxAttempts) || maxAttempts < 1) {
    return { error: "Max attempts must be at least 1." };
  }
  if (!Number.isInteger(timeoutSecs) || timeoutSecs < 1) {
    return { error: "Timeout seconds must be at least 1." };
  }

  return {
    data: {
      name,
      description: form.description.trim(),
      endpoint_url: executionMode === "http" ? endpointURL : undefined,
      cron,
      max_attempts: maxAttempts,
      timeout_secs: timeoutSecs,
      retry_strategy: form.retry_strategy,
      execution_mode: executionMode,
      queue_name: form.queue_name.trim(),
      enabled: form.enabled,
    },
  };
}
