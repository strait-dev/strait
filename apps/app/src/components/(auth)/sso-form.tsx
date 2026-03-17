import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const ssoSchema = z.object({
  email: z.string().email("Enter a valid work email"),
});

type SsoFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

export const SsoForm = ({ redirectTo, disabled }: SsoFormProps) => {
  const form = useForm({
    defaultValues: { email: "" },
    validators: { onChange: ssoSchema },
    onSubmit: async ({ value }) => {
      const { email } = ssoSchema.parse(value);
      const result = await authClient.signIn.sso({
        email,
        callbackURL: redirectTo ?? "/app",
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "sso",
          email,
          provider: "sso",
        });
        toast.error(
          result.error.message ??
            "SSO sign in failed. Your organization may not have SSO configured."
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
              <FieldLabel htmlFor={field.name}>Work email</FieldLabel>
              <Input
                autoComplete="email"
                disabled={disabled}
                id={field.name}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="you@company.com"
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
          Continue with SSO
        </Button>
      </div>
    </form>
  );
};
