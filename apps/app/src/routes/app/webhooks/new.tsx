import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Checkbox } from "@strait/ui/components/checkbox";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { toast } from "@strait/ui/components/toast/index";
import { cn } from "@strait/ui/utils/index";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { useMemo } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import { useCreateWebhook } from "@/hooks/api/use-webhooks";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { formatFieldErrors } from "@/lib/form-errors";
import { ChevronLeftIcon, LoadingIcon, PlusIcon } from "@/lib/icons";
import { tierAtLeast } from "@/lib/plan-tiers";

const BASIC_EVENTS = [
  {
    value: "run.completed",
    label: "Run completed",
    description: "Fired when a job run completes successfully",
  },
  {
    value: "run.failed",
    label: "Run failed",
    description: "Fired when a job run fails",
  },
] as const;

const PRO_EVENTS = [
  {
    value: "run.timed_out",
    label: "Run timed out",
    description: "Fired when a job run exceeds its timeout",
  },
  {
    value: "run.canceled",
    label: "Run canceled",
    description: "Fired when a job run is manually canceled",
  },
  {
    value: "workflow.completed",
    label: "Workflow completed",
    description: "Fired when a workflow run completes",
  },
  {
    value: "workflow.failed",
    label: "Workflow failed",
    description: "Fired when a workflow run fails",
  },
  {
    value: "compute_budget_warning",
    label: "Compute budget warning",
    description: "Fired when compute spend approaches the spending limit",
  },
  {
    value: "slo.budget_warning",
    label: "SLO budget warning",
    description: "Fired when SLO error budget is running low",
  },
] as const;

const createWebhookSchema = z.object({
  webhook_url: z.url("Must be a valid URL"),
  event_types: z.array(z.string()).min(1, "Select at least one event type"),
});

export const Route = createFileRoute("/app/webhooks/new")({
  head: () => ({ meta: [{ title: "New webhook · Strait" }] }),
  errorComponent: ErrorComponent,
  component: CreateWebhookPage,
});

function CreateWebhookPage() {
  const createWebhook = useCreateWebhook();
  const currentPlan = useCurrentPlan();
  const hasProEvents = tierAtLeast(currentPlan, "pro");
  const router = useRouter();

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
          const webhook = await createWebhook.mutateAsync(parsed);
          if (webhook) {
            router.navigate({
              to: "/app/webhooks/$id",
              params: { id: webhook.id },
            });
          } else {
            router.navigate({ to: "/app/webhooks" });
          }
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
    <Shell>
      <div className="mb-6 flex items-center gap-3">
        <Button render={<Link to="/app/webhooks" />} variant="ghost">
          <HugeiconsIcon icon={ChevronLeftIcon} size={14} />
        </Button>
        <h1 className="text-balance font-normal text-xl tracking-tight">
          Create webhook
        </h1>
      </div>

      <form
        className="mx-auto max-w-2xl space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
          form.handleSubmit();
        }}
      >
        <Card>
          <CardHeader>
            <CardTitle>Endpoint</CardTitle>
            <CardDescription>
              The URL that will receive HTTP POST notifications for subscribed
              events.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form.Field name="webhook_url">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>URL</FieldLabel>
                  <Input
                    aria-describedby={
                      field.state.meta.isTouched &&
                      field.state.meta.errors.length > 0
                        ? `${field.name}-error`
                        : undefined
                    }
                    aria-invalid={
                      field.state.meta.isTouched &&
                      field.state.meta.errors.length > 0
                    }
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="https://example.com/webhooks/strait"
                    value={field.state.value}
                  />
                  {field.state.meta.isTouched &&
                    field.state.meta.errors.length > 0 && (
                      <FieldError id={`${field.name}-error`}>
                        {formatFieldErrors(field.state.meta.errors)}
                      </FieldError>
                    )}
                </Field>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Event types</CardTitle>
            <CardDescription>
              Choose which events should trigger a delivery to your endpoint.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form.Field name="event_types">
              {(field) => (
                <Field>
                  <div className="space-y-3">
                    {BASIC_EVENTS.map((event) => (
                      // biome-ignore lint/a11y/noLabelWithoutControl: Checkbox is a custom component wrapping a native input
                      <label
                        className="flex cursor-pointer items-start gap-3 rounded-md border border-transparent px-3 py-2.5 transition-colors hover:bg-muted/50"
                        key={event.value}
                      >
                        <Checkbox
                          checked={field.state.value.includes(event.value)}
                          className="mt-0.5"
                          onCheckedChange={() => toggleEventType(event.value)}
                        />
                        <div>
                          <span className="font-medium text-sm">
                            {event.label}
                          </span>
                          <p className="text-muted-foreground text-xs">
                            {event.description}
                          </p>
                        </div>
                      </label>
                    ))}

                    <div className="my-2 border-t" />

                    {PRO_EVENTS.map((event) => (
                      // biome-ignore lint/a11y/noLabelWithoutControl: Checkbox is a custom component wrapping a native input
                      <label
                        className="flex cursor-pointer items-start gap-3 rounded-md border border-transparent px-3 py-2.5 transition-colors hover:bg-muted/50"
                        key={event.value}
                      >
                        <Checkbox
                          checked={field.state.value.includes(event.value)}
                          className="mt-0.5"
                          disabled={!hasProEvents}
                          onCheckedChange={() => toggleEventType(event.value)}
                        />
                        <div className="flex-1">
                          <span
                            className={cn(
                              "font-medium text-sm",
                              !hasProEvents && "text-muted-foreground"
                            )}
                          >
                            {event.label}
                          </span>
                          {!hasProEvents && (
                            <Badge
                              className="ml-1.5 text-[10px]"
                              variant="outline"
                            >
                              Pro
                            </Badge>
                          )}
                          <p className="text-muted-foreground text-xs">
                            {event.description}
                          </p>
                        </div>
                      </label>
                    ))}
                  </div>
                  {field.state.meta.isTouched &&
                    field.state.meta.errors.length > 0 && (
                      <FieldError className="mt-2" id={`${field.name}-error`}>
                        {formatFieldErrors(field.state.meta.errors)}
                      </FieldError>
                    )}
                </Field>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <div className="flex justify-end gap-3">
          <Button render={<Link to="/app/webhooks" />} variant="secondary">
            Cancel
          </Button>
          <form.Subscribe
            selector={(state) => ({
              canSubmit: state.canSubmit,
              isSubmitting: state.isSubmitting,
            })}
          >
            {({ canSubmit, isSubmitting }) => (
              <Button
                disabled={!canSubmit || isSubmitting || createWebhook.isPending}
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
        </div>
      </form>
    </Shell>
  );
}
