import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Checkbox } from "@strait/ui/components/checkbox";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { InputWithShowHidePassword } from "@strait/ui/components/input-with-show-hide-password";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useState, useTransition } from "react";
import * as z from "zod";
import { authClient } from "@/lib/auth-client";
import { formatFieldErrors } from "@/lib/form-errors";
import { AlertIcon, TrashIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";

type Props = {
  user: AuthUser;
};

const DeleteAccountSchema = z.object({
  email: z.email("Invalid email").min(1, "Email is required"),
  currentPassword: z.string().min(1, "Current password is required"),
  confirmDelete: z.boolean().refine((val) => val === true, {
    message: "You need to confirm that you want to delete your account",
  }),
});

const DeleteAccount = ({ user }: Props) => {
  const [isPending, startTransition] = useTransition();
  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const form = useForm({
    defaultValues: {
      email: "",
      currentPassword: "",
      confirmDelete: false,
    },
    validators: {
      onChange: DeleteAccountSchema,
    },
    onSubmit: ({ value }) => {
      if (value.email !== user.email) {
        toast.error("The email provided does not match your registered email");
        return;
      }
      setIsDialogOpen(true);
    },
  });

  const onDelete = () => {
    startTransition(() => {
      toast.promise(
        authClient.deleteUser({
          password: form.state.values.currentPassword,
          callbackURL: "/",
        }),
        {
          loading: "Deleting account...",
          success: "Account deleted successfully",
          error: "Error deleting account. Please try again.",
        }
      );

      setIsDialogOpen(false);
      form.reset();
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Delete account</CardTitle>
        <CardDescription>
          Permanently delete your account and all associated data
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
            <Alert variant="destructive">
              <HugeiconsIcon className="size-4" icon={AlertIcon} />
              <AlertDescription>
                Warning: This action is irreversible and all your data will be
                permanently lost.
              </AlertDescription>
            </Alert>

            <form.Field name="email">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>
                    Confirm your email
                  </FieldLabel>
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
                    placeholder="Enter your email to confirm"
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

            <form.Field name="currentPassword">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Current password</FieldLabel>
                  <InputWithShowHidePassword
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
                    placeholder="Enter your current password"
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

            <form.Field name="confirmDelete">
              {(field) => (
                <Field className="flex flex-row items-start space-x-3 space-y-0">
                  <div className="flex items-center gap-2">
                    <Checkbox
                      checked={field.state.value}
                      id={field.name}
                      onCheckedChange={(checked) =>
                        field.handleChange(checked === true)
                      }
                    />
                    <FieldLabel htmlFor={field.name}>
                      I confirm that I want to delete my account
                    </FieldLabel>
                  </div>
                </Field>
              )}
            </form.Field>
          </div>
        </CardContent>

        <CardFooter className="flex justify-end gap-3 border-t px-6 py-4">
          <AlertDialog onOpenChange={setIsDialogOpen} open={isDialogOpen}>
            <AlertDialogTrigger
              render={
                <Button
                  className="w-fit"
                  disabled={
                    !(form.state.isDirty && form.state.canSubmit) || isPending
                  }
                  type="submit"
                  variant="destructive"
                />
              }
            >
              {isPending ? (
                <Spinner />
              ) : (
                <HugeiconsIcon className="size-4" icon={TrashIcon} />
              )}
              Delete my account
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  Are you sure you want to delete your account?
                </AlertDialogTitle>
                <AlertDialogDescription>
                  This action is irreversible. This will permanently delete your
                  account and remove all your data from our servers.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <div className="flex justify-end gap-4">
                  <AlertDialogCancel className="w-fit">
                    Cancel
                  </AlertDialogCancel>
                  <AlertDialogAction
                    className="w-fit"
                    disabled={isPending}
                    onClick={onDelete}
                    variant="destructive-solid"
                  >
                    {isPending ? (
                      <Spinner />
                    ) : (
                      <HugeiconsIcon className="size-4" icon={TrashIcon} />
                    )}
                    Yes, delete my account
                  </AlertDialogAction>
                </div>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardFooter>
      </form>
    </Card>
  );
};

export default DeleteAccount;
