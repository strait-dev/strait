import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const forgotPasswordSchema = z.object({
  email: z.string().email("Invalid email address"),
});

type ForgotPasswordFormProps = {
  disabled?: boolean;
};

export const ForgotPasswordForm = ({ disabled }: ForgotPasswordFormProps) => {
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
        <div className="rounded-full bg-primary/10 p-3">
          <svg
            aria-hidden="true"
            className="size-6 text-primary"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            viewBox="0 0 24 24"
          >
            <path
              d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>
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
                autoComplete="email"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="you@example.com"
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

        <Button
          className="w-full"
          disabled={disabled || form.state.isSubmitting}
          type="submit"
        >
          {form.state.isSubmitting ? (
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
          ) : null}
          Send reset link
        </Button>
      </div>
    </form>
  );
};
