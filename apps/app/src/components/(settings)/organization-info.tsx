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
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useTransition } from "react";
import { z } from "zod/v4";
import {
  organizationQueryOptions,
  useUpdateOrganization,
} from "@/hooks/auth/use-organization";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PencilEditIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

const orgFormSchema = z.object({
  name: z.string().min(1, "Organization name is required"),
  slug: z.string(),
  description: z.string(),
  email: z.string(),
  website: z.string(),
  phone: z.string(),
});

interface OrganizationInfoProps {
  organizationId: string;
}

const OrganizationInfo = ({ organizationId }: OrganizationInfoProps) => {
  const [isSubmitting, startTransition] = useTransition();
  const { data: organization, isLoading } = useQuery(
    organizationQueryOptions(organizationId)
  );
  const updateOrganization = useUpdateOrganization();

  const form = useForm({
    defaultValues: {
      name: organization?.name ?? "",
      slug: organization?.slug ?? "",
      description: "",
      email: "",
      website: "",
      phone: "",
    },
    validators: { onChange: orgFormSchema },
    onSubmit: ({ value }) => {
      startTransition(() => {
        try {
          toast.promise(
            updateOrganization.mutateAsync({
              organizationId,
              name: value.name,
              slug: value.slug || undefined,
            }),
            {
              loading: "Updating organization...",
              success: "Organization updated successfully!",
              error: (err: unknown) => {
                captureException(err);
                return "Failed to update organization.";
              },
            }
          );
        } catch (error) {
          captureException(error);
        }
      });
    },
  });

  const isProcessing = isSubmitting || updateOrganization.isPending;

  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-8">
          <div className="flex items-center justify-center gap-2 text-muted-foreground text-sm">
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
            Loading organization...
          </div>
        </CardContent>
      </Card>
    );
  }

  if (!organization) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Organization Details</CardTitle>
        <CardDescription>
          Update your organization's name and information.
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
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <form.Field name="name">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Name</FieldLabel>
                    <Input
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="Organization name"
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

              <form.Field name="slug">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Slug</FieldLabel>
                    <Input
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="organization-slug"
                      type="text"
                      value={field.state.value ?? ""}
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

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <form.Field name="email">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Email</FieldLabel>
                    <Input
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="org@example.com"
                      type="email"
                      value={field.state.value ?? ""}
                    />
                    {field.state.meta.errors.length > 0 && (
                      <FieldError>
                        {formatFieldErrors(field.state.meta.errors)}
                      </FieldError>
                    )}
                  </Field>
                )}
              </form.Field>

              <form.Field name="website">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Website</FieldLabel>
                    <Input
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="https://example.com"
                      type="url"
                      value={field.state.value ?? ""}
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

            <form.Field name="description">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Description</FieldLabel>
                  <Textarea
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="A brief description of your organization"
                    value={field.state.value ?? ""}
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

export default OrganizationInfo;
