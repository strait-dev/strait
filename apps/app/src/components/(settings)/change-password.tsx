import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { PasswordInput } from "@strait/ui/components/password-input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { captureException } from "@/lib/sentry";

const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, "Current password is required"),
    newPassword: z.string().min(8, "Password must be at least 8 characters"),
    confirmPassword: z.string().min(1, "Please confirm your password"),
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: "Passwords don't match",
    path: ["confirmPassword"],
  });

const ChangePassword = () => {
  const form = useForm({
    defaultValues: {
      currentPassword: "",
      newPassword: "",
      confirmPassword: "",
    },
    validators: { onChange: changePasswordSchema },
    onSubmit: async ({ value }) => {
      try {
        const result = await authClient.changePassword({
          currentPassword: value.currentPassword,
          newPassword: value.newPassword,
          revokeOtherSessions: true,
        });

        if (result.error) {
          toast.error(result.error.message ?? "Failed to change password.");
          return;
        }

        toast.success("Password changed successfully.");
        form.reset();
      } catch (error) {
        captureException(error);
        toast.error("Something went wrong while changing your password.");
      }
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Change password</CardTitle>
        <CardDescription>
          Update your password. You'll be signed out of other devices.
        </CardDescription>
      </CardHeader>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          e.stopPropagation();
          form.handleSubmit();
        }}
      >
        <CardContent className="pb-6">
          <div className="flex flex-col gap-4">
            <form.Field name="currentPassword">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Current password</FieldLabel>
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
                    id={field.name}
                    onBlur={field.handleBlur}
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Enter current password"
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

            <form.Field name="newPassword">
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

            <form.Field name="confirmPassword">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Confirm password</FieldLabel>
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
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Re-enter new password"
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
          </div>
        </CardContent>

        <CardFooter className="flex justify-end border-t px-6 py-4">
          <form.Subscribe
            selector={(state) => ({
              canSubmit: state.canSubmit,
              isDirty: state.isDirty,
              isSubmitting: state.isSubmitting,
            })}
          >
            {({ canSubmit, isDirty, isSubmitting }) => (
              <Button
                className="w-fit"
                disabled={!isDirty || isSubmitting || !canSubmit}
                type="submit"
              >
                {isSubmitting ? <Spinner /> : null}
                Change password
              </Button>
            )}
          </form.Subscribe>
        </CardFooter>
      </form>
    </Card>
  );
};

export default ChangePassword;
