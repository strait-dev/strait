import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PasswordInput } from "@strait/ui/components/password-input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import { getPostHog } from "@/lib/analytics";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { MailIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";
import { consumeUtmParams, utmToSetOnce } from "@/lib/utm";

const signUpSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.email("Invalid email address"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

type SignUpFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

const SignUpForm = ({ redirectTo, disabled }: SignUpFormProps) => {
  const form = useForm({
    defaultValues: { name: "", email: "", password: "" },
    validators: { onMount: signUpSchema, onChange: signUpSchema },
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
        throw new Error(result.error.message ?? "Failed to create account");
      }

      const utm = consumeUtmParams();
      const setOnce: Record<string, string> = {
        initial_signup_date: new Date().toISOString(),
        ...(utm ? utmToSetOnce(utm) : {}),
      };
      getPostHog()?.capture("auth_signed_up", {
        method: "email",
        $set_once: setOnce,
      });
    },
  });

  return (
    <form.Subscribe selector={(state) => state.isSubmitSuccessful}>
      {(isSubmitSuccessful) =>
        isSubmitSuccessful ? (
          <div className="flex flex-col items-center gap-3 py-4 text-center">
            <EmptyMedia media="icon" size="lg" variant="info">
              <HugeiconsIcon className="size-6" icon={MailIcon} />
            </EmptyMedia>
            <p className="font-medium text-foreground text-sm">
              Verify your email
            </p>
            <p className="text-muted-foreground text-sm">
              We sent a verification link to your email. Click the link to
              activate your account.
            </p>
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              e.stopPropagation();
              form.handleSubmit().catch(() => undefined);
            }}
          >
            <div className="flex flex-col gap-4">
              <form.Field name="name">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Full name</FieldLabel>
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
                      autoComplete="name"
                      disabled={disabled}
                      id={field.name}
                      name={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
                      placeholder="Enter your full name"
                      type="text"
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
                    <FieldLabel htmlFor={field.name}>Password</FieldLabel>
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
                      disabled={disabled}
                      id={field.name}
                      name={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
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
                    Create account
                  </Button>
                )}
              </form.Subscribe>
            </div>
          </form>
        )
      }
    </form.Subscribe>
  );
};

export default SignUpForm;
