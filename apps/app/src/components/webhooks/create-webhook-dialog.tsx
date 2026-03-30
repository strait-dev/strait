import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
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
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useMemo } from "react";
import { z } from "zod/v4";
import { useCreateWebhook } from "@/hooks/api/use-webhooks";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PlusIcon } from "@/lib/icons";
import { tierAtLeast } from "@/lib/plan-tiers";

const BASIC_EVENTS = [
  { value: "run.completed", label: "Run completed" },
  { value: "run.failed", label: "Run failed" },
] as const;

const PRO_EVENTS = [
  { value: "run.timed_out", label: "Run timed out" },
  { value: "run.canceled", label: "Run canceled" },
  { value: "workflow.completed", label: "Workflow completed" },
  { value: "workflow.failed", label: "Workflow failed" },
  { value: "compute_budget_warning", label: "Compute budget warning" },
  { value: "slo.budget_warning", label: "SLO budget warning" },
] as const;

const createWebhookSchema = z.object({
  webhook_url: z.url("Must be a valid URL"),
  event_types: z.array(z.string()).min(1, "Select at least one event type"),
});

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CreateWebhookDialog = ({ open, onOpenChange }: Props) => {
  const createWebhook = useCreateWebhook();
  const currentPlan = useCurrentPlan();
  const hasProEvents = tierAtLeast(currentPlan, "pro");

  const defaultValues = useMemo(
    () => ({
      webhook_url: "",
      event_types: [] as string[],
    }),
    []
  );

  const form = useForm({
    defaultValues,
    validators: { onChange: createWebhookSchema },
    onSubmit: ({ value }) => {
      const parsed = createWebhookSchema.parse(value);

      toast.promise(
        (async () => {
          await createWebhook.mutateAsync(parsed);
          form.reset();
          onOpenChange(false);
        })(),
        {
          loading: "Creating webhook...",
          success: "Webhook created successfully!",
          error: "Failed to create webhook. Please try again.",
        }
      );
    },
  });

  function toggleEventType(eventType: string) {
    const current = form.getFieldValue("event_types");
    if (current.includes(eventType)) {
      form.setFieldValue(
        "event_types",
        current.filter((t: string) => t !== eventType)
      );
    } else {
      form.setFieldValue("event_types", [...current, eventType]);
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Create webhook</DialogTitle>
            <DialogDescription>
              Subscribe to events and receive HTTP POST notifications at your
              endpoint.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-4">
            <form.Field name="webhook_url">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Endpoint URL</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="https://example.com/webhooks"
                    value={field.state.value}
                  />
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>
                      {formatFieldErrors(field.state.meta.errors)}
                    </FieldError>
                  )}
                </Field>
              )}
            </form.Field>

            <form.Field name="event_types">
              {(field) => (
                <Field>
                  <FieldLabel>Event types</FieldLabel>
                  <div className="space-y-2">
                    {BASIC_EVENTS.map((event) => (
                      // biome-ignore lint/a11y/noLabelWithoutControl: Checkbox is a custom component wrapping a native input
                      <label
                        className="flex cursor-pointer items-center gap-2 text-sm"
                        key={event.value}
                      >
                        <Checkbox
                          checked={field.state.value.includes(event.value)}
                          onCheckedChange={() => toggleEventType(event.value)}
                        />
                        {event.label}
                      </label>
                    ))}

                    {PRO_EVENTS.map((event) => (
                      // biome-ignore lint/a11y/noLabelWithoutControl: Checkbox is a custom component wrapping a native input
                      <label
                        className="flex cursor-pointer items-center gap-2 text-sm"
                        key={event.value}
                      >
                        <Checkbox
                          checked={field.state.value.includes(event.value)}
                          disabled={!hasProEvents}
                          onCheckedChange={() => toggleEventType(event.value)}
                        />
                        <span
                          className={
                            hasProEvents ? "" : "text-muted-foreground"
                          }
                        >
                          {event.label}
                        </span>
                        {!hasProEvents && (
                          <Badge className="text-[10px]" variant="outline">
                            Pro
                          </Badge>
                        )}
                      </label>
                    ))}
                  </div>
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>
                      {formatFieldErrors(field.state.meta.errors)}
                    </FieldError>
                  )}
                </Field>
              )}
            </form.Field>
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Cancel
            </DialogClose>
            <form.Subscribe
              selector={(state) => ({
                canSubmit: state.canSubmit,
                isSubmitting: state.isSubmitting,
              })}
            >
              {({ canSubmit, isSubmitting }) => (
                <Button
                  disabled={
                    !canSubmit || isSubmitting || createWebhook.isPending
                  }
                  type="submit"
                >
                  {isSubmitting || createWebhook.isPending ? (
                    <HugeiconsIcon
                      className="size-4 animate-spin"
                      icon={LoadingIcon}
                    />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  Create webhook
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export default CreateWebhookDialog;
