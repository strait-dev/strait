import { HugeiconsIcon } from "@hugeicons/react";
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
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon } from "@/lib/icons";
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
          form.handleSubmit();
        }}
      >
        <CardContent>
          <div className="flex flex-col gap-4">
            <form.Field name="currentPassword">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Current password</FieldLabel>
                  <PasswordInput
                    autoComplete="current-password"
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Enter current password"
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

            <form.Field name="newPassword">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>New password</FieldLabel>
                  <PasswordInput
                    autoComplete="new-password"
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

            <form.Field name="confirmPassword">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Confirm password</FieldLabel>
                  <PasswordInput
                    autoComplete="new-password"
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Re-enter new password"
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
          </div>
        </CardContent>

        <CardFooter className="flex justify-end">
          <Button
            className="w-fit"
            disabled={
              !form.state.isDirty ||
              form.state.isSubmitting ||
              !form.state.canSubmit
            }
            type="submit"
          >
            {form.state.isSubmitting ? (
              <HugeiconsIcon
                className="size-4 animate-spin"
                icon={LoadingIcon}
              />
            ) : null}
            Change password
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
};

export default ChangePassword;
