import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

const magicLinkSchema = z.object({
  email: z.string().email("Invalid email address"),
});

type MagicLinkFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

export const MagicLinkForm = ({ redirectTo, disabled }: MagicLinkFormProps) => {
  const [sent, setSent] = useState(false);

  const form = useForm({
    defaultValues: { email: "" },
    validators: { onChange: magicLinkSchema },
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
          We sent a sign-in link to your email address. Click the link to sign
          in.
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
                <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
              )}
            </Field>
          )}
        </form.Field>

        <Button
          className="w-full"
          disabled={disabled || form.state.isSubmitting}
          size="lg"
          type="submit"
        >
          {form.state.isSubmitting ? (
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
          ) : null}
          Send magic link
        </Button>
      </div>
    </form>
  );
};
