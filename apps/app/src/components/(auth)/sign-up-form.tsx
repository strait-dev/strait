import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PasswordInput } from "@strait/ui/components/password-input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const signUpSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.string().email("Invalid email address"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

type SignUpFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

const SignUpForm = ({ redirectTo, disabled }: SignUpFormProps) => {
  const [emailSent, setEmailSent] = useState(false);

  const form = useForm({
    defaultValues: { name: "", email: "", password: "" },
    validators: { onChange: signUpSchema },
    onSubmit: async ({ value }) => {
      const { name, email, password } = signUpSchema.parse(value);
      const result = await authClient.signUp.email({
        name,
        email,
        password,
        callbackURL: redirectTo ?? "/app",
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "signup",
          email,
          provider: "email",
        });
        toast.error(
          result.error.message ?? "Failed to create account. Please try again."
        );
        return;
      }

      setEmailSent(true);
    },
  });

  if (emailSent) {
    return (
      <div className="flex flex-col items-center gap-3 py-4 text-center">
        <div className="rounded-lg bg-muted p-3">
          <svg
            aria-hidden="true"
            className="size-6 text-foreground"
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
        <p className="font-medium text-foreground text-sm">Verify your email</p>
        <p className="text-muted-foreground text-sm">
          We sent a verification link to your email. Click the link to activate
          your account.
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
        <form.Field name="name">
          {(field) => (
            <Field className="w-full">
              <FieldLabel htmlFor={field.name}>Full name</FieldLabel>
              <Input
                autoComplete="name"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="Enter your full name"
                type="text"
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

        <form.Field name="password">
          {(field) => (
            <Field className="w-full">
              <FieldLabel htmlFor={field.name}>Password</FieldLabel>
              <PasswordInput
                autoComplete="new-password"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="At least 8 characters"
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
          Create account
        </Button>
      </div>
    </form>
  );
};

export default SignUpForm;
