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
import { Input } from "@strait/ui/components/input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useRef, useTransition } from "react";
import { z } from "zod";
import { useUpdateUser } from "@/hooks/auth/use-user";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { PencilEditIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";
import type { AuthUser } from "@/routes/__root";

const userFormSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.email("Invalid email"),
  phone: z.string().optional(),
});

type Props = {
  user: AuthUser;
};

const PersonalInfo = ({ user }: Props) => {
  const [isSubmitting, startTransition] = useTransition();
  const originalEmail = useRef(user.email);

  const updateCurrentUser = useUpdateUser();

  const form = useForm({
    defaultValues: userFormSchema.parse(user) as z.input<typeof userFormSchema>,
    validators: {
      onChange: userFormSchema,
    },
    onSubmit: ({ value }) => {
      const values = userFormSchema.parse(value);
      const emailChanged = values.email !== originalEmail.current;

      startTransition(() => {
        try {
          toast.promise(
            updateCurrentUser.mutateAsync(values).then(async () => {
              if (emailChanged) {
                const result = await authClient.changeEmail({
                  newEmail: values.email,
                  callbackURL: "/verify-email",
                });
                if (result.error) {
                  throw new Error(
                    result.error.message ?? "Failed to change email"
                  );
                }
              }
            }),
            {
              loading: "Updating data...",
              success: emailChanged
                ? "Data updated! Check your new email for a verification link."
                : "Data updated successfully!",
              error: (err: unknown) => {
                captureException(err);
                return "Something went wrong while updating your data";
              },
            }
          );
        } catch (error) {
          captureException(error);
        }
      });
    },
  });

  const isProcessing = isSubmitting || updateCurrentUser.isPending;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Your data</CardTitle>
        <CardDescription>Update your personal information</CardDescription>
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
                    id={field.name}
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
                    id={field.name}
                    onBlur={field.handleBlur}
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Enter your email"
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
          </div>
        </CardContent>

        <CardFooter className="flex justify-end gap-3 border-t px-6 py-4">
          <form.Subscribe
            selector={(state) => ({
              canSubmit: state.canSubmit,
              isDirty: state.isDirty,
              isSubmitting: state.isSubmitting,
            })}
          >
            {({ canSubmit, isDirty, isSubmitting }) => (
              <>
                <Button
                  className="w-fit"
                  disabled={!isDirty || isProcessing}
                  onClick={() => {
                    if (!isProcessing) {
                      form.reset();
                    }
                  }}
                  type="button"
                  variant="secondary"
                >
                  Cancel
                </Button>

                <Button
                  className="w-fit"
                  disabled={
                    !isDirty || isSubmitting || !canSubmit || isProcessing
                  }
                  type="submit"
                >
                  {isProcessing ? (
                    <Spinner />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PencilEditIcon} />
                  )}
                  Save changes
                </Button>
              </>
            )}
          </form.Subscribe>
        </CardFooter>
      </form>
    </Card>
  );
};

export default PersonalInfo;
