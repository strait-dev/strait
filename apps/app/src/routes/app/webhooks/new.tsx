import { HugeiconsIcon } from "@hugeicons/react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  CardCheckboxGroup,
  CardCheckboxItem,
} from "@strait/ui/components/card-checkbox";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { SecretInput } from "@strait/ui/components/secret-input";
import { Separator } from "@strait/ui/components/separator";
import { Shell } from "@strait/ui/components/shell";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import {
  type CreateWebhookResult,
  useCreateWebhook,
} from "@/hooks/api/use-webhooks";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { formatFieldErrors } from "@/lib/form-errors";
import { CheckIcon, ChevronLeftIcon, PlusIcon } from "@/lib/icons";
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
    value: "slo.budget_warning",
    label: "SLO budget warning",
    description: "Fired when SLO error budget is running low",
  },
  {
    value: "billing.cap_warning",
    label: "Billing cap warning",
    description: "Fired when an organization reaches 80% of its spending cap",
  },
  {
    value: "billing.cap_reached",
    label: "Billing cap reached",
    description: "Fired when an organization reaches 100% of its spending cap",
  },
  {
    value: "billing.cap_disabled",
    label: "Billing cap disabled",
    description: "Fired when an organization removes its spending cap",
  },
  {
    value: "billing.overage_disabled",
    label: "Billing overage disabled",
    description: "Fired when an organization disables paid-plan overage",
  },
  {
    value: "billing.suspended",
    label: "Billing suspended",
    description: "Fired when an organization is suspended after dunning",
  },
  {
    value: "billing.delinquent",
    label: "Billing delinquent",
    description: "Fired when a suspended organization moves to delinquent",
  },
  {
    value: "billing.payment_succeeded",
    label: "Billing payment succeeded",
    description: "Fired when an overdue payment succeeds",
  },
  {
    value: "schedule.suspended",
    label: "Schedule suspended",
    description: "Fired when billing or plan limits pause a schedule",
  },
  {
    value: "workflow.registration_rejected",
    label: "Workflow registration rejected",
    description: "Fired when a launch plan gate rejects workflow registration",
  },
  {
    value: "sla.credit_issued",
    label: "SLA credit issued",
    description: "Fired when an SLA service credit is recorded",
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
  const [createdWebhook, setCreatedWebhook] =
    useState<CreateWebhookResult | null>(null);

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
          const result = await createWebhook.mutateAsync(parsed);
          setCreatedWebhook(result);
          form.reset();
        })(),
        {
          loading: "Creating webhook...",
          success: "Webhook created. Copy the signing secret now.",
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

      {createdWebhook && (
        <Alert className="mx-auto mb-6 max-w-2xl" variant="success">
          <HugeiconsIcon className="size-4" icon={CheckIcon} />
          <AlertTitle>Webhook created</AlertTitle>
          <AlertDescription>
            Copy this signing secret now. It will not be shown again.
            <SecretInput
              aria-label="Webhook signing secret"
              className="mt-3 font-mono"
              readOnly
              value={createdWebhook.signing_secret}
            />
          </AlertDescription>
          <div className="col-start-2 mt-4 flex flex-wrap justify-end gap-3">
            <Button
              onClick={() =>
                router.navigate({
                  to: "/app/webhooks/$id",
                  params: { id: createdWebhook.subscription.id },
                })
              }
            >
              View webhook
            </Button>
          </div>
        </Alert>
      )}

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
                  <CardCheckboxGroup>
                    {BASIC_EVENTS.map((event) => (
                      <CardCheckboxItem
                        checked={field.state.value.includes(event.value)}
                        description={event.description}
                        id={`event-type-${event.value}`}
                        key={event.value}
                        label={event.label}
                        layout="start"
                        onCheckedChange={() => toggleEventType(event.value)}
                      />
                    ))}

                    <Separator className="my-1" />

                    {PRO_EVENTS.map((event) => (
                      <CardCheckboxItem
                        checked={field.state.value.includes(event.value)}
                        description={event.description}
                        disabled={!hasProEvents}
                        id={`event-type-${event.value}`}
                        key={event.value}
                        label={
                          <>
                            {event.label}
                            {!hasProEvents && (
                              <Badge className="ml-1.5" variant="outline">
                                Pro
                              </Badge>
                            )}
                          </>
                        }
                        layout="start"
                        onCheckedChange={() => toggleEventType(event.value)}
                      />
                    ))}
                  </CardCheckboxGroup>
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
                  <Spinner />
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
