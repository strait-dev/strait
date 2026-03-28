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
import { useNavigate } from "@tanstack/react-router";
import { useMemo } from "react";
import { z } from "zod/v4";

import type { Agent } from "@/hooks/api/types";
import { useCreateAgent } from "@/hooks/api/use-agents";
import { queryKeys } from "@/hooks/query-keys";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PlusIcon } from "@/lib/icons";

const createAgentSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  slug: z
    .string()
    .min(2, "Slug must be at least 2 characters")
    .regex(
      /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
      "Slug must use lowercase letters, numbers, and hyphens only"
    ),
  description: z.string(),
  model: z.string().min(2, "Model is required"),
  configText: z.string(),
});

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
};

function slugifyName(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}

const CreateAgentDialog = ({ open, onOpenChange, projectId }: Props) => {
  const createAgent = useCreateAgent();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const defaultValues = useMemo(
    () => ({
      name: "",
      slug: "",
      description: "",
      model: "gpt-5.4",
      configText: '{\n  "temperature": 0.2\n}',
    }),
    []
  );

  const form = useForm({
    defaultValues,
    validators: { onChange: createAgentSchema },
    onSubmit: ({ value }) => {
      const parsed = createAgentSchema.parse(value);

      let config: unknown;
      const trimmedConfig = parsed.configText.trim();
      if (trimmedConfig !== "") {
        try {
          config = JSON.parse(trimmedConfig);
        } catch {
          form.setFieldMeta("configText", (meta) => ({
            ...meta,
            errors: ["Config must be valid JSON"],
          }));
          return;
        }
      }

      toast.promise(
        (async () => {
          const agent = (await createAgent.mutateAsync({
            projectId,
            name: parsed.name,
            slug: parsed.slug,
            description: parsed.description,
            model: parsed.model,
            config,
          })) as Agent;

          await queryClient.invalidateQueries({
            queryKey: queryKeys.agents._def,
          });
          form.reset();
          onOpenChange(false);
          navigate({
            params: { id: agent.id },
            to: "/app/agents/$id",
          });
        })(),
        {
          loading: "Creating agent...",
          success: "Agent created successfully!",
          error: "Failed to create agent. Please try again.",
        }
      );
    },
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-lg">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Create agent</DialogTitle>
            <DialogDescription>
              Create a managed agent in the current project, then deploy and run
              it locally from the dashboard.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-4">
            <form.Field name="name">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Name</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(event) => {
                      const name = event.target.value;
                      field.handleChange(name);
                      const nextSlug = slugifyName(name);
                      const currentSlug = form.getFieldValue("slug");
                      if (
                        !currentSlug ||
                        currentSlug === slugifyName(field.state.value)
                      ) {
                        form.setFieldValue("slug", nextSlug);
                      }
                    }}
                    placeholder="Support triage agent"
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

            <form.Field name="slug">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Slug</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                    placeholder="support-triage-agent"
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

            <form.Field name="model">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Model</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                    placeholder="gpt-5.4"
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
                <Field>
                  <FieldLabel htmlFor={field.name}>
                    Description (optional)
                  </FieldLabel>
                  <Textarea
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                    placeholder="Summarizes and routes incoming support issues."
                    rows={3}
                    value={field.state.value}
                  />
                </Field>
              )}
            </form.Field>

            <form.Field name="configText">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Config JSON</FieldLabel>
                  <Textarea
                    className="min-h-32 font-mono text-xs"
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                    placeholder='{\n  "temperature": 0.2\n}'
                    rows={8}
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
                  disabled={!canSubmit || isSubmitting || createAgent.isPending}
                  type="submit"
                >
                  {isSubmitting || createAgent.isPending ? (
                    <HugeiconsIcon
                      className="size-4 animate-spin"
                      icon={LoadingIcon}
                    />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  Create agent
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export default CreateAgentDialog;
