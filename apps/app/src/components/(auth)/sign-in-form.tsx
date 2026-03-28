import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PasswordInput } from "@strait/ui/components/password-input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { z } from "zod";
import { getPostHog } from "@/lib/analytics";
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

async function waitForClientSession(): Promise<void> {
  for (let attempt = 0; attempt < 10; attempt += 1) {
    const sessionResult = await authClient.getSession();
    if (sessionResult.data?.session?.token) {
      return;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 100));
  }

  throw new Error("Timed out waiting for browser session after sign-in");
}

const SignInForm = ({
  redirectTo,
  disabled,
  onTwoFactorRequired,
}: SignInFormProps) => {
  const [isHydrated, setIsHydrated] = useState(false);
  const [emailNotVerified, setEmailNotVerified] = useState(false);
  const [unverifiedEmail, setUnverifiedEmail] = useState("");
  const [isResending, setIsResending] = useState(false);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  const form = useForm({
    defaultValues: { email: "", password: "" },
    validators: { onChange: signInSchema },
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
        await waitForClientSession();
        window.location.assign(redirectTo ?? "/app");
        return;
      }

      if (result.error) {
        if (result.error.status === 403 && onTwoFactorRequired) {
          onTwoFactorRequired();
          return;
        }

        const message = result.error.message ?? "";

        // Handle unverified email
        if (
          message.toLowerCase().includes("email is not verified") ||
          result.error.code === "EMAIL_NOT_VERIFIED"
        ) {
          setEmailNotVerified(true);
          setUnverifiedEmail(email);
          return;
        }

        // Handle wrong provider (user signed up with OAuth, trying email/password)
        if (
          message.toLowerCase().includes("no credential account") ||
          message.toLowerCase().includes("invalid credentials") ||
          result.error.code === "INVALID_PASSWORD"
        ) {
          // Check if user exists with social account
          toast.error(
            "Invalid email or password. If you signed up with Google or GitHub, use that method instead."
          );
          return;
        }

        captureSentryAuthError(result.error, {
          operation: "email-signin",
          email,
          provider: "email",
        });
        toast.error(message || "Failed to sign in. Please try again.");
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
          Your email address hasn't been verified yet. Check your inbox for a
          verification link.
        </p>
        <Button
          disabled={isResending}
          onClick={handleResendVerification}
          variant="outline"
        >
          {isResending ? (
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
          ) : null}
          Resend verification email
        </Button>
        <button
          className="text-foreground text-sm underline-offset-4 hover:underline"
          onClick={() => setEmailNotVerified(false)}
          type="button"
        >
          Try again
        </button>
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
                disabled={disabled || !isHydrated}
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
                disabled={disabled || !isHydrated}
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
          disabled={disabled || form.state.isSubmitting || !isHydrated}
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

export default SignInForm;
