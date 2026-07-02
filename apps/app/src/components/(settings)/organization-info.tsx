import { HugeiconsIcon } from "@hugeicons/react";
import { Avatar, AvatarFallback } from "@strait/ui/components/avatar";
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
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useTransition } from "react";
import { z } from "zod/v4";
import {
  organizationQueryOptions,
  useUpdateOrganization,
} from "@/hooks/auth/use-organization";
import { queryKeys } from "@/hooks/query-keys";
import { formatFieldErrors } from "@/lib/form-errors";
import { PencilEditIcon, StoreIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

const orgFormSchema = z.object({
  name: z.string().min(1, "Organization name is required"),
  slug: z.string(),
  description: z.string(),
  email: z.string(),
  website: z.string(),
});

type OrgMetadata = {
  description?: string;
  email?: string;
  website?: string;
};

function parseMetadata(metadata: unknown): OrgMetadata {
  if (metadata && typeof metadata === "object") {
    const value = (metadata as { value?: unknown }).value;
    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        return parsed && typeof parsed === "object"
          ? (parsed as OrgMetadata)
          : {};
      } catch {
        return {};
      }
    }
    return metadata as OrgMetadata;
  }
  return {};
}

interface OrganizationInfoProps {
  canEdit?: boolean;
  organizationId: string;
}

const OrganizationInfo = ({
  canEdit = true,
  organizationId,
}: OrganizationInfoProps) => {
  const [isSubmitting, startTransition] = useTransition();
  const queryClient = useQueryClient();
  const { data: organization, isLoading } = useQuery(
    organizationQueryOptions(organizationId)
  );
  const updateOrganization = useUpdateOrganization();

  const meta = parseMetadata(organization?.metadata);

  const form = useForm({
    defaultValues: {
      name: organization?.name ?? "",
      slug: organization?.slug ?? "",
      description: meta.description ?? "",
      email: meta.email ?? "",
      website: meta.website ?? "",
    },
    validators: { onChange: orgFormSchema },
    onSubmit: ({ value }) => {
      const metadata = {
        ...parseMetadata(organization?.metadata),
        description: value.description || undefined,
        email: value.email || undefined,
        website: value.website || undefined,
      };

      startTransition(() => {
        try {
          toast.promise(
            updateOrganization
              .mutateAsync({
                organizationId,
                name: value.name,
                slug: value.slug || undefined,
                metadata,
              })
              .then(() => {
                queryClient.invalidateQueries({
                  queryKey:
                    queryKeys.organizations.detail(organizationId).queryKey,
                });
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

  // Sync form when organization data loads or changes
  useEffect(() => {
    if (organization) {
      const orgMeta = parseMetadata(organization.metadata);
      form.reset({
        name: organization.name ?? "",
        slug: organization.slug ?? "",
        description: orgMeta.description ?? "",
        email: orgMeta.email ?? "",
        website: orgMeta.website ?? "",
      });
    }
  }, [organization, form]);

  const isProcessing = isSubmitting || updateOrganization.isPending;

  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-8">
          <div className="flex items-center justify-center gap-2 text-muted-foreground text-sm">
            <Spinner />
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
        <CardTitle>Organization details</CardTitle>
        <CardDescription>
          Update your organization's name and public information.
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
          <div className="flex flex-col gap-6">
            <div className="flex items-center gap-3">
              <Avatar className="size-16">
                <AvatarFallback className="text-lg">
                  <HugeiconsIcon className="size-6" icon={StoreIcon} />
                </AvatarFallback>
              </Avatar>
              <div className="flex flex-col gap-1">
                <span className="font-medium text-sm">{organization.name}</span>
                <span className="text-muted-foreground text-sm">
                  Logo uploads are not available yet.
                </span>
              </div>
            </div>

            {/* Name & Slug */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
                      disabled={!canEdit}
                      id={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
                      placeholder="Organization name"
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

              <form.Field name="slug">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Slug</FieldLabel>
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
                      disabled={!canEdit}
                      id={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
                      placeholder="organization-slug"
                      type="text"
                      value={field.state.value ?? ""}
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

            {/* Email & Website */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
                      disabled={!canEdit}
                      id={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
                      placeholder="org@example.com"
                      type="email"
                      value={field.state.value ?? ""}
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

              <form.Field name="website">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Website</FieldLabel>
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
                      disabled={!canEdit}
                      id={field.name}
                      onBlur={field.handleBlur}
                      onInput={(e) => field.handleChange(e.currentTarget.value)}
                      placeholder="https://example.com"
                      type="url"
                      value={field.state.value ?? ""}
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

            {/* Description */}
            <form.Field name="description">
              {(field) => (
                <Field className="w-full">
                  <FieldLabel htmlFor={field.name}>Description</FieldLabel>
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
                    disabled={!canEdit}
                    id={field.name}
                    onBlur={field.handleBlur}
                    onInput={(e) => field.handleChange(e.currentTarget.value)}
                    placeholder="A brief description of your organization"
                    value={field.state.value ?? ""}
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

        {canEdit && (
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
        )}
      </form>
    </Card>
  );
};

export default OrganizationInfo;
