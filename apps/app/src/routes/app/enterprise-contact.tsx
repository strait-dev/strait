import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { Spinner } from "@strait/ui/components/spinner";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useMemo } from "react";
import ErrorComponent from "@/components/common/error-component";
import {
  enterpriseContactSchema,
  MONTHLY_SPEND_RANGES,
  TEAM_SIZES,
  USE_CASES,
} from "@/lib/enterprise-contact-schema";
import { formatFieldErrors } from "@/lib/form-errors";
import { ChevronLeftIcon } from "@/lib/icons";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import { getResend } from "@/lib/resend.server";
import { authMiddleware } from "@/middlewares/auth";

const submitEnterpriseContact = createServerFn({ method: "POST" })
  .inputValidator(enterpriseContactSchema)
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    await enforceRateLimit({
      key: `enterprise-contact:${context.user.id}`,
      limit: 5,
      windowSeconds: 86_400,
    });

    const email = context.user.email;
    if (!email) {
      throw new Error("Authenticated email is required");
    }

    const resend = getResend();
    await resend.emails.send({
      from: "Enterprise <hello@usestrait.com>",
      to: "leo@strait.dev",
      subject: `Enterprise inquiry from ${data.company}`,
      text: [
        `Name: ${data.name}`,
        `Account email: ${email}`,
        `Company: ${data.company}`,
        `Team size: ${data.teamSize}`,
        ...(data.useCase ? [`Use case: ${data.useCase}`] : []),
        ...(data.expectedSpend
          ? [`Expected spend: ${data.expectedSpend}`]
          : []),
        `Message: ${data.message}`,
      ].join("\n"),
    });
    return { success: true };
  });

export const Route = createFileRoute("/app/enterprise-contact")({
  head: () => ({ meta: [{ title: "Contact sales · Strait" }] }),
  errorComponent: ErrorComponent,
  component: EnterpriseContactPage,
});

function EnterpriseContactPage() {
  const router = useRouter();

  const defaultValues = useMemo(
    () => ({
      name: "",
      email: "",
      company: "",
      teamSize: "",
      useCase: "",
      expectedSpend: "",
      message: "",
    }),
    []
  );

  const form = useForm({
    defaultValues,
    validators: { onChange: enterpriseContactSchema },
    onSubmit: ({ value }) => {
      const parsed = enterpriseContactSchema.parse(value);

      toast.promise(
        (async () => {
          await submitEnterpriseContact({ data: parsed });
          router.navigate({ to: "/app/upgrade" });
        })(),
        {
          loading: "Sending your inquiry...",
          success:
            "Your inquiry has been sent. Our team will be in touch shortly.",
          error: "Failed to send inquiry. Please try again.",
        }
      );
    },
  });

  return (
    <Shell>
      <div className="mb-6 flex items-center gap-3">
        <Button render={<Link to="/app/upgrade" />} variant="ghost">
          <HugeiconsIcon icon={ChevronLeftIcon} size={14} />
        </Button>
        <h1 className="text-balance font-normal text-xl tracking-tight">
          Contact enterprise sales
        </h1>
      </div>

      <form
        className="mx-auto max-w-2xl space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
          e.stopPropagation();
          form.handleSubmit();
        }}
      >
        <Card>
          <CardHeader>
            <CardTitle>Your details</CardTitle>
            <CardDescription>
              Tell us about yourself and your team so we can tailor a plan for
              you.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <form.Field name="name">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Name</FieldLabel>
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
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Jane Smith"
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

            <form.Field name="email">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Email</FieldLabel>
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
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="jane@company.com"
                    type="email"
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

            <form.Field name="company">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Company</FieldLabel>
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
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Acme Inc."
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

            <form.Field name="teamSize">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Team size</FieldLabel>
                  <Select
                    onValueChange={(val) => field.handleChange(val as string)}
                    value={field.state.value}
                  >
                    <SelectTrigger
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
                      className="w-full"
                      id={field.name}
                    >
                      <SelectValue placeholder="Select team size" />
                    </SelectTrigger>
                    <SelectContent>
                      {TEAM_SIZES.map((size) => (
                        <SelectItem key={size} value={size}>
                          {size}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {field.state.meta.isTouched &&
                    field.state.meta.errors.length > 0 && (
                      <FieldError id={`${field.name}-error`}>
                        {formatFieldErrors(field.state.meta.errors)}
                      </FieldError>
                    )}
                </Field>
              )}
            </form.Field>
            <form.Field name="useCase">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>
                    Primary use case (optional)
                  </FieldLabel>
                  <Select
                    onValueChange={(val) => field.handleChange(val as string)}
                    value={field.state.value}
                  >
                    <SelectTrigger className="w-full" id={field.name}>
                      <SelectValue placeholder="Select use case" />
                    </SelectTrigger>
                    <SelectContent>
                      {USE_CASES.map((uc) => (
                        <SelectItem key={uc} value={uc}>
                          {uc}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}
            </form.Field>

            <form.Field name="expectedSpend">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>
                    Expected monthly spend (optional)
                  </FieldLabel>
                  <Select
                    onValueChange={(val) => field.handleChange(val as string)}
                    value={field.state.value}
                  >
                    <SelectTrigger className="w-full" id={field.name}>
                      <SelectValue placeholder="Select range" />
                    </SelectTrigger>
                    <SelectContent>
                      {MONTHLY_SPEND_RANGES.map((range) => (
                        <SelectItem key={range} value={range}>
                          {range}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Message</CardTitle>
            <CardDescription>
              Describe your use case, requirements, or any questions you have
              about the Enterprise plan.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form.Field name="message">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Message</FieldLabel>
                  <Textarea
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
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Tell us about your infrastructure needs..."
                    rows={5}
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

        <div className="flex justify-end gap-3">
          <Button render={<Link to="/app/upgrade" />} variant="secondary">
            Cancel
          </Button>
          <form.Subscribe
            selector={(state) => ({
              canSubmit: state.canSubmit,
              isSubmitting: state.isSubmitting,
            })}
          >
            {({ canSubmit, isSubmitting }) => (
              <Button disabled={!canSubmit || isSubmitting} type="submit">
                {isSubmitting ? <Spinner /> : null}
                Send inquiry
              </Button>
            )}
          </form.Subscribe>
        </div>
      </form>
    </Shell>
  );
}
