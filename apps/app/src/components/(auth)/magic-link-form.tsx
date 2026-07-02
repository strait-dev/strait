import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import { getPostHog } from "@/lib/analytics";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { MailIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const magicLinkSchema = z.object({
  email: z.email("Invalid email address"),
});

type MagicLinkFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

const MagicLinkForm = ({ redirectTo, disabled }: MagicLinkFormProps) => {
  const form = useForm({
    defaultValues: { email: "" },
    validators: { onMount: magicLinkSchema, onChange: magicLinkSchema },
    onSubmit: async ({ value }) => {
      const { email } = magicLinkSchema.parse(value);

      const result = await authClient.signIn.magicLink({
        email,
        callbackURL: redirectTo ?? "/app",
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "magic-link",
          email,
          provider: "magic-link",
        });
        toast.error(
          result.error.message ?? "Failed to send magic link. Please try again."
        );
        throw new Error(result.error.message ?? "Failed to send magic link");
      }

      getPostHog()?.capture("auth_signed_in", { method: "magic_link" });
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
              Check your email
            </p>
            <p className="text-muted-foreground text-sm">
              We sent a sign-in link to your email address. Click the link to
              sign in.
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
                    Send magic link
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

export default MagicLinkForm;
