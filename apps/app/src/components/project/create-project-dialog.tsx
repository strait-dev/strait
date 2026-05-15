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
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useMemo } from "react";
import { z } from "zod/v4";
import {
  useCreateProject,
  useSetActiveProject,
} from "@/hooks/api/use-projects";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PlusIcon } from "@/lib/icons";

const createProjectSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  description: z.string(),
});

type Props = {
  organizationId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CreateProjectDialog = ({ organizationId, open, onOpenChange }: Props) => {
  const createProject = useCreateProject();
  const setActiveProject = useSetActiveProject();
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
    validators: { onChange: createProjectSchema },
    onSubmit: ({ value }) => {
      const parsed = createProjectSchema.parse(value);

      toast.promise(
        (async () => {
          const project = await createProject.mutateAsync({
            organizationId,
            name: parsed.name,
            description: parsed.description,
          });

          if (project) {
            await setActiveProject.mutateAsync({ projectId: project.id });
            await queryClient.invalidateQueries();
            router.invalidate();
          }

          form.reset();
          onOpenChange(false);
        })(),
        {
          loading: "Creating project...",
          success: "Project created successfully!",
          error: "Failed to create project. Please try again.",
        }
      );
    },
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <form
          onSubmit={(e) => {
            e.preventDefault();
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
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="My Project"
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
                    onChange={(e) => field.handleChange(e.target.value)}
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
                  disabled={
                    !canSubmit || isSubmitting || createProject.isPending
                  }
                  type="submit"
                >
                  {isSubmitting || createProject.isPending ? (
                    <HugeiconsIcon
                      className="size-4 animate-spin"
                      icon={LoadingIcon}
                    />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  Create project
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
