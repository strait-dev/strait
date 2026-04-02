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
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useMemo } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import { formatFieldErrors } from "@/lib/form-errors";
import { ChevronLeftIcon, LoadingIcon } from "@/lib/icons";
import { getResend } from "@/lib/resend.server";
import { authMiddleware } from "@/middlewares/auth";

const TEAM_SIZES = ["1-10", "11-50", "51-200", "201-500", "500+"] as const;

const enterpriseContactSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.email("Must be a valid email address"),
  company: z.string().min(1, "Company name is required"),
  teamSize: z.string().min(1, "Team size is required"),
  message: z.string().min(10, "Message must be at least 10 characters"),
});

const submitEnterpriseContact = createServerFn({ method: "POST" })
  .inputValidator(enterpriseContactSchema)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const resend = getResend();
    await resend.emails.send({
      from: "Enterprise <hello@usestrait.com>",
      to: "leo@strait.dev",
      subject: `Enterprise inquiry from ${data.company}`,
      text: [
        `Name: ${data.name}`,
        `Email: ${data.email}`,
        `Company: ${data.company}`,
        `Team Size: ${data.teamSize}`,
        `Message: ${data.message}`,
      ].join("\n"),
    });
    return { success: true };
  });

export const Route = createFileRoute("/app/enterprise-contact")({
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
        <Button render={<Link to="/app/upgrade" />} size="sm" variant="ghost">
          <HugeiconsIcon icon={ChevronLeftIcon} size={14} />
        </Button>
        <h1 className="text-balance font-semibold text-lg">
          Contact Enterprise Sales
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
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Jane Smith"
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

            <form.Field name="email">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Email</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="jane@company.com"
                    type="email"
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

            <form.Field name="company">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Company</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Acme Inc."
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

            <form.Field name="teamSize">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Team size</FieldLabel>
                  <Select
                    onValueChange={(val) => field.handleChange(val as string)}
                    value={field.state.value}
                  >
                    <SelectTrigger className="w-full">
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
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>
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
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Tell us about your infrastructure needs..."
                    rows={5}
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
                {isSubmitting ? (
                  <HugeiconsIcon
                    className="size-4 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : null}
                Send inquiry
              </Button>
            )}
          </form.Subscribe>
        </div>
      </form>
    </Shell>
  );
}
