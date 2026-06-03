import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { MailIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const forgotPasswordSchema = z.object({
  email: z.email("Invalid email address"),
});

type ForgotPasswordFormProps = {
  disabled?: boolean;
};

const ForgotPasswordForm = ({ disabled }: ForgotPasswordFormProps) => {
  const [sent, setSent] = useState(false);

  const form = useForm({
    defaultValues: { email: "" },
    validators: { onChange: forgotPasswordSchema },
    onSubmit: async ({ value }) => {
      const { email } = forgotPasswordSchema.parse(value);
      const result = await authClient.requestPasswordReset({
        email,
        redirectTo: "/reset-password",
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "password-reset-request",
          email,
          provider: "email",
        });
        toast.error(
          result.error.message ??
            "Failed to send reset email. Please try again."
        );
        return;
      }

      setSent(true);
    },
  });

  if (sent) {
    return (
      <div className="flex flex-col items-center gap-3 py-4 text-center">
        <EmptyMedia media="icon" size="lg" variant="info">
          <HugeiconsIcon className="size-6" icon={MailIcon} />
        </EmptyMedia>
        <p className="font-medium text-foreground text-sm">Check your email</p>
        <p className="text-muted-foreground text-sm">
          If an account exists with that email, we sent a password reset link.
        </p>
      </div>
    );
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        form.handleSubmit();
      }}
    >
      <div className="flex flex-col gap-4">
        <form.Field name="email">
          {(field) => (
            <Field className="w-full">
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
                autoComplete="email"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="you@example.com"
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

        <Button
          className="w-full"
          disabled={disabled || form.state.isSubmitting}
          type="submit"
          variant="brand-solid"
        >
          {form.state.isSubmitting ? <Spinner /> : null}
          Send reset link
        </Button>
      </div>
    </form>
  );
};

export default ForgotPasswordForm;
