import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PasswordInput } from "@strait/ui/components/password-input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { z } from "zod";
import { getPostHog } from "@/lib/analytics";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { MailIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const signInSchema = z.object({
  email: z.email("Invalid email address"),
  password: z.string().min(1, "Password is required"),
});

type SignInFormProps = {
  redirectTo?: string;
  disabled?: boolean;
  onTwoFactorRequired?: () => void;
};

type SignInAuthError = {
  code?: string;
  message?: string;
  status?: number;
};

type SignInErrorHandlers = {
  onEmailNotVerified: () => void;
  onTwoFactorRequired?: () => void;
};

function handleSignInError(
  error: SignInAuthError,
  email: string,
  { onEmailNotVerified, onTwoFactorRequired }: SignInErrorHandlers
): never {
  if (error.status === 403 && onTwoFactorRequired) {
    onTwoFactorRequired();
    throw new Error(error.message ?? "Two-factor required");
  }

  const message = error.message ?? "";

  // Handle unverified email
  if (
    message.toLowerCase().includes("email is not verified") ||
    error.code === "EMAIL_NOT_VERIFIED"
  ) {
    onEmailNotVerified();
    throw new Error(message || "Email is not verified");
  }

  // Handle wrong provider (user signed up with OAuth, trying email/password)
  if (
    message.toLowerCase().includes("no credential account") ||
    message.toLowerCase().includes("invalid credentials") ||
    error.code === "INVALID_PASSWORD"
  ) {
    // Check if user exists with social account
    toast.error(
      "Invalid email or password. If you signed up with Google or GitHub, use that method instead."
    );
    throw new Error(message || "Invalid email or password");
  }

  captureSentryAuthError(error, {
    operation: "email-signin",
    email,
    provider: "email",
  });
  toast.error(message || "Failed to sign in. Please try again.");
  throw new Error(message || "Failed to sign in");
}

const SignInForm = ({
  redirectTo,
  disabled,
  onTwoFactorRequired,
}: SignInFormProps) => {
  const [emailNotVerified, setEmailNotVerified] = useState(false);
  const [unverifiedEmail, setUnverifiedEmail] = useState("");
  const [isResending, setIsResending] = useState(false);

  const form = useForm({
    defaultValues: { email: "", password: "" },
    validators: { onMount: signInSchema, onChange: signInSchema },
    onSubmit: async ({ value }) => {
      const { email, password } = signInSchema.parse(value);
      setEmailNotVerified(false);

      const result = await authClient.signIn.email({
        email,
        password,
        callbackURL: redirectTo ?? "/app",
      });

      if (!result.error) {
        getPostHog()?.capture("auth_signed_in", { method: "email" });
      }

      if (result.error) {
        handleSignInError(result.error, email, {
          onTwoFactorRequired,
          onEmailNotVerified: () => {
            setEmailNotVerified(true);
            setUnverifiedEmail(email);
          },
        });
      }
    },
  });

  const handleResendVerification = async () => {
    setIsResending(true);
    try {
      await authClient.sendVerificationEmail({
        email: unverifiedEmail,
        callbackURL: "/verify-email",
      });
      toast.success("Verification email sent. Check your inbox.");
    } catch {
      toast.error("Failed to resend verification email.");
    } finally {
      setIsResending(false);
    }
  };

  if (emailNotVerified) {
    return (
      <div className="flex flex-col items-center gap-3 py-4 text-center">
        <EmptyMedia media="icon" size="lg" variant="info">
          <HugeiconsIcon className="size-6" icon={MailIcon} />
        </EmptyMedia>
        <p className="font-medium text-foreground text-sm">Verify your email</p>
        <p className="text-muted-foreground text-sm">
          Your email address hasn't been verified yet. Check your inbox for a
          verification link.
        </p>
        <Button
          disabled={isResending}
          onClick={handleResendVerification}
          variant="secondary-outline"
        >
          {isResending ? <Spinner /> : null}
          Resend verification email
        </Button>
        <Button
          onClick={() => setEmailNotVerified(false)}
          type="button"
          variant="link"
        >
          Try again
        </Button>
      </div>
    );
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        e.stopPropagation();
        form.handleSubmit().catch(() => undefined);
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
                name={field.name}
                onBlur={field.handleBlur}
                onInput={(e) => field.handleChange(e.currentTarget.value)}
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
                autoComplete="current-password"
                disabled={disabled}
                id={field.name}
                name={field.name}
                onBlur={field.handleBlur}
                onInput={(e) => field.handleChange(e.currentTarget.value)}
                placeholder="Enter your password"
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

        <form.Subscribe
          selector={(state) => ({
            canSubmit: state.canSubmit,
            isSubmitting: state.isSubmitting,
          })}
        >
          {({ canSubmit, isSubmitting }) => (
            <Button
              className="w-full"
              disabled={disabled || !canSubmit || isSubmitting}
              type="submit"
              variant="brand-solid"
            >
              {isSubmitting ? <Spinner /> : null}
              Sign in
            </Button>
          )}
        </form.Subscribe>
      </div>
    </form>
  );
};

export default SignInForm;
