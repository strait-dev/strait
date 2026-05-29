import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { PasswordInput } from "@strait/ui/components/password-input";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { useState } from "react";
import * as z from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const resetPasswordSearchSchema = z.object({
  token: z.string(),
  error: z.string().optional().catch(undefined),
});

const resetPasswordSchema = z
  .object({
    password: z.string().min(8, "Password must be at least 8 characters"),
    confirmPassword: z.string().min(1, "Please confirm your password"),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: "Passwords don't match",
    path: ["confirmPassword"],
  });

export const Route = createFileRoute("/(auth)/reset-password")({
  head: () => ({ meta: [{ title: "Reset password · Strait" }] }),
  validateSearch: resetPasswordSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated && !search.token) {
      throw redirect({ to: "/app" });
    }
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: ResetPasswordPage,
});

function ResetPasswordPage() {
  const { token, error: searchError } = Route.useSearch();
  const [success, setSuccess] = useState(false);

  const form = useForm({
    defaultValues: { password: "", confirmPassword: "" },
    validators: { onChange: resetPasswordSchema },
    onSubmit: async ({ value }) => {
      const result = await authClient.resetPassword({
        newPassword: value.password,
        token,
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "password-reset",
          provider: "email",
        });
        toast.error(
          result.error.message ??
            "Failed to reset password. The link may have expired."
        );
        return;
      }

      setSuccess(true);
    },
  });

  return (
    <AuthLayout title="Set new password">
      {searchError ? (
        <div
          className="rounded-md bg-destructive/10 p-3 text-destructive text-sm"
          role="alert"
        >
          {searchError === "INVALID_TOKEN"
            ? "This reset link is invalid or has expired. Please request a new one."
            : searchError}
        </div>
      ) : null}

      {success ? (
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <p className="font-medium text-foreground text-sm">
            Password reset successfully
          </p>
          <p className="text-muted-foreground text-sm">
            You can now sign in with your new password.
          </p>
          <Link
            className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 font-medium text-primary-foreground text-sm hover:bg-primary/90"
            to="/login"
          >
            Back to sign in
          </Link>
        </div>
      ) : (
        <>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              form.handleSubmit();
            }}
          >
            <div className="flex flex-col gap-4">
              <form.Field name="password">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>New password</FieldLabel>
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
                      autoComplete="new-password"
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="At least 8 characters"
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

              <form.Field name="confirmPassword">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>
                      Confirm password
                    </FieldLabel>
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
                      autoComplete="new-password"
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="Re-enter your password"
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
                disabled={form.state.isSubmitting}
                type="submit"
              >
                {form.state.isSubmitting ? (
                  <HugeiconsIcon
                    className="size-4 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : null}
                Reset password
              </Button>
            </div>
          </form>

          <p className="text-center text-muted-foreground text-sm">
            <Link
              className="text-foreground underline-offset-4 hover:underline"
              to="/login"
            >
              Back to sign in
            </Link>
          </p>
        </>
      )}
    </AuthLayout>
  );
}
