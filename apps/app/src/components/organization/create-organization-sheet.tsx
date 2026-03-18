import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  type Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { nanoid } from "nanoid";
import { useMemo } from "react";
import { z } from "zod/v4";
import { useCreateOrganization } from "@/hooks/auth/use-organization";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import { ORGANIZATION_SLUG_LENGTH } from "@/utils/constants";

const insertOrganizationSchema = z.object({
  name: z.string().min(1, "Organization name is required"),
  description: z.string(),
});

type Props = React.ComponentPropsWithRef<typeof Sheet> & {
  onClose: () => void;
  user: AuthUser;
};

const CreateOrganizationSheet = ({ onClose, user }: Props) => {
  const createOrganization = useCreateOrganization();

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

      toast.promise(
        createOrganization.mutateAsync({ name: value.name, slug }),
        {
          loading: "Creating organization...",
          success: () => {
            form.reset();
            onClose();
            return "Organization created successfully!";
          },
          error:
            "Error creating organization. Please try again. If the problem persists, contact support.",
        }
      );
    },
  });

  return (
    <SheetContent className="w-[400px] sm:w-[500px] sm:max-w-[500px]">
      <form
        className="flex h-full flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          form.handleSubmit();
        }}
      >
        <SheetHeader className="px-4">
          <SheetTitle>Create new organization</SheetTitle>
          <SheetDescription>
            Create an organization to collaborate with your team. You can invite
            members and manage projects within it.
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-4 px-4">
          <form.Field name="name">
            {(field) => (
              <Field className="w-full">
                <FieldLabel htmlFor={field.name}>Name</FieldLabel>
                <Input
                  id={field.name}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  placeholder="Enter the organization name"
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

          <form.Field name="description">
            {(field) => (
              <Field className="w-full">
                <FieldLabel htmlFor={field.name}>
                  Description (optional)
                </FieldLabel>
                <Textarea
                  id={field.name}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  placeholder="What does this organization do?"
                  rows={3}
                  value={field.state.value}
                />
              </Field>
            )}
          </form.Field>
        </div>

        <SheetFooter className="bottom-0 mt-auto flex w-full gap-2 px-4 pt-4">
          <SheetClose
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
          </SheetClose>

          <form.Subscribe
            selector={(state) => ({
              isDirty: state.isDirty,
              isSubmitting: state.isSubmitting,
              canSubmit: state.canSubmit,
            })}
          >
            {({ isDirty, isSubmitting, canSubmit }) => (
              <Button
                className="w-full"
                disabled={
                  !isDirty ||
                  isSubmitting ||
                  !canSubmit ||
                  createOrganization.isPending
                }
                type="submit"
              >
                {isSubmitting || createOrganization.isPending ? (
                  <HugeiconsIcon
                    className="size-4 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : (
                  <HugeiconsIcon className="size-4" icon={PlusIcon} />
                )}
                Create organization
              </Button>
            )}
          </form.Subscribe>
        </SheetFooter>
      </form>
    </SheetContent>
  );
};

export default CreateOrganizationSheet;
