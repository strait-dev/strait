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
import { z } from "zod/v4";
import type { Project } from "@/hooks/api/types";
import { useCreateAndActivateProject } from "@/hooks/api/use-projects";
import { formatFieldErrors } from "@/lib/form-errors";
import { PlusIcon } from "@/lib/icons";

const createProjectSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  description: z.string(),
});

type Props = {
  organizationId: string;
  open: boolean;
  onCreated?: (project: Project) => void;
  onOpenChange: (open: boolean) => void;
};

const CreateProjectDialog = ({
  organizationId,
  open,
  onCreated,
  onOpenChange,
}: Props) => {
  const createProject = useCreateAndActivateProject();

  const defaultValues = {
    name: "",
    description: "",
  };

  const form = useForm({
    defaultValues,
    validators: { onChange: createProjectSchema },
    onSubmit: async ({ value }) => {
      const parsed = createProjectSchema.parse(value);

      try {
        const project = await toast.promise(
          createProject.mutateAsync({
            organizationId,
            name: parsed.name,
            description: parsed.description,
          }),
          {
            loading: "Creating project...",
            success: "Project created successfully!",
            error: "Failed to create project. Please try again.",
          }
        );

        onCreated?.(project);
        form.reset();
        onOpenChange(false);
      } catch {
        // handled by toast
      }
    },
  });

  const isPending = createProject.isPending;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            e.stopPropagation();
            form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Create new project</DialogTitle>
            <DialogDescription>
              Projects organize your jobs, workflows, and runs within an
              organization.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-4">
            <form.Field name="name">
              {(field) => (
                <Field>
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
                    placeholder="My project"
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
                <Field>
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
                    placeholder="What is this project for?"
                    rows={3}
                    value={field.state.value}
                  />
                </Field>
              )}
            </form.Field>
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Cancel
            </DialogClose>
            <form.Subscribe
              selector={(state) => ({
                canSubmit: state.canSubmit,
                isSubmitting: state.isSubmitting,
              })}
            >
              {({ canSubmit, isSubmitting }) => (
                <Button
                  disabled={!canSubmit || isSubmitting || isPending}
                  type="submit"
                >
                  {isSubmitting || isPending ? (
                    <Spinner />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  {isSubmitting || isPending
                    ? "Creating project..."
                    : "Create project"}
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export default CreateProjectDialog;
