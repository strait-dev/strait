import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { Spinner } from "@strait/ui/components/spinner";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { nanoid } from "nanoid";
import { useMemo } from "react";
import { z } from "zod/v4";
import {
  useCreateOrganization,
  useSetDefaultOrganization,
} from "@/hooks/auth/use-organization";
import { formatFieldErrors } from "@/lib/form-errors";
import { PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import { ORGANIZATION_SLUG_LENGTH } from "@/utils/constants";

const insertOrganizationSchema = z.object({
  name: z.string().min(1, "Organization name is required"),
  description: z.string(),
});

type Props = {
  onClose: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  user: AuthUser;
};

const CreateOrganizationDialog = ({
  onClose,
  onOpenChange,
  open,
  user,
}: Props) => {
  const createOrganization = useCreateOrganization();
  const setDefaultOrganization = useSetDefaultOrganization();
  const queryClient = useQueryClient();
  const router = useRouter();

  const defaultValues = useMemo(
    () => ({
      name: "",
      description: "",
    }),
    []
  );

  const form = useForm({
    defaultValues,
    validators: { onChange: insertOrganizationSchema },
    onSubmit: ({ value }) => {
      if (!user.id) {
        toast.error("User not found, please contact support.");
        return;
      }

      const slug = `${value.name.toLowerCase().replace(/\s+/g, "-")}-${nanoid(ORGANIZATION_SLUG_LENGTH)}`;

      const createPromise = (async () => {
        const org = await createOrganization.mutateAsync({
          name: value.name,
          slug,
        });

        if (org?.id) {
          await setDefaultOrganization.mutateAsync({ id: org.id });
        }

        await queryClient.invalidateQueries();
        router.invalidate();

        return org;
      })();

      toast.promise(createPromise, {
        loading: "Creating organization...",
        success: () => {
          form.reset();
          onClose();
          return "Organization created successfully!";
        },
        error:
          "Error creating organization. Please try again. If the problem persists, contact support.",
      });
    },
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-[480px]">
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            e.stopPropagation();
            form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Create new organization</DialogTitle>
            <DialogDescription>
              Create an organization to collaborate with your team. You can
              invite members and manage projects within it.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4">
            <form.Field name="name">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Name</FieldLabel>
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
                    autoFocus
                    id={field.name}
                    onBlur={field.handleBlur}
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="Enter the organization name"
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

            <form.Field name="description">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>
                    Description (optional)
                  </FieldLabel>
                  <Textarea
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
                    placeholder="What does this organization do?"
                    rows={3}
                    value={field.state.value}
                  />
                </Field>
              )}
            </form.Field>
          </div>

          <DialogFooter className="gap-2">
            <DialogClose
              render={
                <Button
                  onClick={() => {
                    onClose();
                    form.reset();
                  }}
                  variant="secondary"
                />
              }
            >
              Cancel
            </DialogClose>

            <form.Subscribe
              selector={(state) => ({
                isDirty: state.isDirty,
                isSubmitting: state.isSubmitting,
                canSubmit: state.canSubmit,
              })}
            >
              {({ isDirty, isSubmitting, canSubmit }) => (
                <Button
                  disabled={
                    !isDirty ||
                    isSubmitting ||
                    !canSubmit ||
                    createOrganization.isPending
                  }
                  type="submit"
                >
                  {isSubmitting || createOrganization.isPending ? (
                    <Spinner />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  Create organization
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export default CreateOrganizationDialog;
