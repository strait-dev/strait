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
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useTransition } from "react";
import { z } from "zod";
import { useUpdateUser } from "@/hooks/auth/use-user";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PencilEditIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";
import type { AuthUser } from "@/routes/__root";

const userFormSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.string().email("Invalid email"),
  phone: z.string().optional(),
});

type Props = {
  user: AuthUser;
};

const PersonalInfo = ({ user }: Props) => {
  const [isSubmitting, startTransition] = useTransition();

  const updateCurrentUser = useUpdateUser();

  const form = useForm({
    defaultValues: userFormSchema.parse(user) as z.input<typeof userFormSchema>,
    validators: {
      onChange: userFormSchema,
    },
    onSubmit: ({ value }) => {
      const values = userFormSchema.parse(value);
      startTransition(() => {
        try {
          toast.promise(updateCurrentUser.mutateAsync(values), {
            loading: "Updating data...",
            success: "Data updated successfully!",
            error: (err: unknown) => {
              captureException(err);
              return "Something went wrong while updating your data";
            },
          });
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
          form.handleSubmit();
        }}
      >
        <CardContent>
          <div className="flex flex-col gap-4">
            <form.Field name="name">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Full Name</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Enter your full name"
                    type="text"
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

            <form.Field name="email">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Email</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="Enter your email"
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
          </div>
        </CardContent>

        <CardFooter className="flex justify-end gap-4">
          <Button
            className="w-fit"
            disabled={!form.state.isDirty || isProcessing}
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
              !form.state.isDirty ||
              form.state.isSubmitting ||
              !form.state.canSubmit ||
              isProcessing
            }
            type="submit"
          >
            {isProcessing ? (
              <HugeiconsIcon
                className="size-4 animate-spin"
                icon={LoadingIcon}
              />
            ) : (
              <HugeiconsIcon className="size-4" icon={PencilEditIcon} />
            )}
            Save changes
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
};

export default PersonalInfo;
