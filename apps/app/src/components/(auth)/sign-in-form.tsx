import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PasswordInput } from "@strait/ui/components/password-input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { Link } from "@tanstack/react-router";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const signInSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(1, "Password is required"),
});

type SignInFormProps = {
  redirectTo?: string;
  disabled?: boolean;
  onTwoFactorRequired?: () => void;
};

export const SignInForm = ({
  redirectTo,
  disabled,
  onTwoFactorRequired,
}: SignInFormProps) => {
  const form = useForm({
    defaultValues: { email: "", password: "" },
    validators: { onChange: signInSchema },
    onSubmit: async ({ value }) => {
      const { email, password } = signInSchema.parse(value);
      const result = await authClient.signIn.email({
        email,
        password,
        callbackURL: redirectTo ?? "/app",
      });

      if (result.error) {
        if (result.error.status === 403 && onTwoFactorRequired) {
          onTwoFactorRequired();
          return;
        }
        captureSentryAuthError(result.error, {
          operation: "email-signin",
          email,
          provider: "email",
        });
        toast.error(
          result.error.message ?? "Failed to sign in. Please try again."
        );
      }
    },
  });

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

        <form.Field name="password">
          {(field) => (
            <Field className="w-full">
              <div className="flex items-center justify-between">
                <FieldLabel htmlFor={field.name}>Password</FieldLabel>
                <Link
                  className="text-foreground text-sm underline-offset-4 hover:underline"
                  to="/forgot-password"
                >
                  Forgot password?
                </Link>
              </div>
              <PasswordInput
                autoComplete="current-password"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="Enter your password"
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
          Sign in
        </Button>
      </div>
    </form>
  );
};
