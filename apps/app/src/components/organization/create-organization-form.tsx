import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Separator } from "@strait/ui/components/separator";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { Link, useNavigate } from "@tanstack/react-router";
import { nanoid } from "nanoid";
import { useId, useMemo } from "react";
import { z } from "zod";
import { UnsavedChangesDialog } from "@/components/common/unsaved-changes-dialog";
import { useCreateOrganization } from "@/hooks/auth/use-organization";
import { useUnsavedChangesWarning } from "@/hooks/use-unsaved-changes-warning";
import { LoadingIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import { ORGANIZATION_SLUG_LENGTH } from "@/utils/constants";
import {
  activities,
  company_size,
  fiscal_types,
  segments,
} from "@/utils/options";

const insertOrganizationSchema = z.object({
  id: z.string().optional(),
  name: z.string().min(1, "Store name is required"),
  slug: z.string().optional(),
  logo: z.string().optional(),
  metadata: z.string().optional(),
  email: z.string().optional(),
  phone: z.string().optional(),
  description: z.string().optional(),
  status: z.enum(["active", "inactive"]).optional(),
  currencyCode: z.string().optional(),
  website: z.string().optional(),
  entityType: z.string().optional(),
  taxId: z.string().optional(),
  businessName: z.string().optional(),
  businessRegistration: z.string().optional(),
  industryCode: z.string().optional(),
  activity: z.string().optional(),
  segment: z.string().optional(),
  size: z.string().optional(),
  employeesSize: z.string().optional(),
  address: z.string().optional(),
  postalCode: z.string().optional(),
  city: z.string().optional(),
  state: z.string().optional(),
  houseNumber: z.string().optional(),
  neighborhood: z.string().optional(),
  complement: z.string().optional(),
  country: z.string().optional(),
  annualRevenue: z.number().optional(),
  primaryGoals: z.string().optional(),
  userId: z.string().optional(),
});

type Props = {
  user: AuthUser;
};

const CreateOrganizationForm = ({ user }: Props) => {
  const entityTypeSelectId = useId();
  const activitySelectId = useId();
  const segmentSelectId = useId();
  const companySizeSelectId = useId();

  const navigate = useNavigate();

  const createOrganization = useCreateOrganization();

  const defaultValues: z.infer<typeof insertOrganizationSchema> = useMemo(
    () => ({
      id: nanoid(),
      name: "",
      slug: "",
      logo: "",
      metadata: "",
      email: "",
      phone: "",
      description: "",
      status: "active",
      currencyCode: "USD",
      website: "",

      // Entity Information
      entityType: "company",

      // Business Information
      taxId: "",
      businessName: "",
      businessRegistration: "",
      industryCode: "",
      activity: "other",
      segment: "commercial",
      size: "micro",
      employeesSize: "less_than_5",

      address: "",
      postalCode: "",
      city: "",
      state: "",
      houseNumber: "",
      neighborhood: "",
      complement: "",

      country: "Brazil",

      // Onboarding Data
      annualRevenue: undefined,
      primaryGoals: "",

      userId: user.id,
    }),
    [user.id]
  );

  const form = useForm({
    defaultValues,
    validators: { onChange: insertOrganizationSchema },
    onSubmit: ({ value }) => {
      if (!user.id) {
        toast.error("User not found, please contact support.");
        return;
      }

      const data: z.input<typeof insertOrganizationSchema> = {
        ...value,
        logo: "",
        metadata: "",
        email: "",
        phone: "",
        description: "",
        slug: `${value.name.toLowerCase().replace(/\s+/g, "-")}-${nanoid(ORGANIZATION_SLUG_LENGTH)}`,
      };

      const parsedValues = insertOrganizationSchema.parse(data);

      toast.promise(createOrganization.mutateAsync(parsedValues), {
        loading: "Creating store...",
        success: () => {
          form.reset();

          navigate({
            to: "/app",
            search: { subscription: undefined, t: undefined },
          });

          return "Store created successfully!";
        },
        error:
          "Error creating store. Please try again. If the problem persists, contact support.",
      });
    },
  });

  const { isBlocked, proceed, reset } = useUnsavedChangesWarning({
    isDirty: form.state.isDirty,
    disabled: createOrganization.isPending || form.state.isSubmitting,
    onDiscard: () => form.reset(),
  });

  return (
    <>
      <form
        className="flex h-fit flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          form.handleSubmit();
        }}
      >
        <div className="flex-1 overflow-hidden">
          <div className="px-2">
            {/* Information */}
            <div className="flex flex-col gap-4">
              <form.Field name="name">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={field.name}>Name</FieldLabel>
                    <Input
                      id={field.name}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                      placeholder="Enter the store name"
                      type="text"
                      value={field.state.value ?? ""}
                    />
                    {field.state.meta.errors.length > 0 && (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    )}
                  </Field>
                )}
              </form.Field>
            </div>

            <Separator className="mt-6 mb-6" />

            {/* Tax Information */}
            <div className="flex flex-col gap-4">
              <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                <form.Field name="entityType">
                  {(field) => (
                    <Field>
                      <FieldLabel htmlFor={entityTypeSelectId}>
                        Entity Type
                      </FieldLabel>
                      <Select
                        onValueChange={(val) =>
                          field.handleChange(val as typeof field.state.value)
                        }
                        value={field.state.value ?? undefined}
                      >
                        <SelectTrigger id={entityTypeSelectId}>
                          <SelectValue placeholder="Select entity type" />
                        </SelectTrigger>
                        <SelectContent>
                          {fiscal_types.map((type) => (
                            <SelectItem key={type.value} value={type.value}>
                              {type.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {field.state.meta.errors.length > 0 && (
                        <FieldError>
                          {field.state.meta.errors.join(", ")}
                        </FieldError>
                      )}
                    </Field>
                  )}
                </form.Field>

                <form.Field name="activity">
                  {(field) => (
                    <Field>
                      <FieldLabel htmlFor={activitySelectId}>
                        Activity
                      </FieldLabel>
                      <Select
                        onValueChange={(val) =>
                          field.handleChange(val as typeof field.state.value)
                        }
                        value={field.state.value ?? undefined}
                      >
                        <SelectTrigger id={activitySelectId}>
                          <SelectValue placeholder="Select an activity" />
                        </SelectTrigger>
                        <SelectContent>
                          {activities.map((activity) => (
                            <SelectItem
                              key={activity.value}
                              value={activity.value}
                            >
                              {activity.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {field.state.meta.errors.length > 0 && (
                        <FieldError>
                          {field.state.meta.errors.join(", ")}
                        </FieldError>
                      )}
                    </Field>
                  )}
                </form.Field>
              </div>

              <form.Field name="segment">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={segmentSelectId}>Segment</FieldLabel>
                    <Select
                      onValueChange={(val) =>
                        field.handleChange(val as typeof field.state.value)
                      }
                      value={field.state.value ?? undefined}
                    >
                      <SelectTrigger id={segmentSelectId}>
                        <SelectValue placeholder="Select a segment" />
                      </SelectTrigger>
                      <SelectContent>
                        {segments.map((segment) => (
                          <SelectItem key={segment.value} value={segment.value}>
                            {segment.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {field.state.meta.errors.length > 0 && (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    )}
                  </Field>
                )}
              </form.Field>

              <form.Field name="size">
                {(field) => (
                  <Field className="w-full">
                    <FieldLabel htmlFor={companySizeSelectId}>
                      Company Size
                    </FieldLabel>
                    <Select
                      onValueChange={(val) =>
                        field.handleChange(val as typeof field.state.value)
                      }
                      value={field.state.value ?? undefined}
                    >
                      <SelectTrigger id={companySizeSelectId}>
                        <SelectValue placeholder="Select the company size" />
                      </SelectTrigger>
                      <SelectContent>
                        {company_size.map((size) => (
                          <SelectItem key={size.value} value={size.value}>
                            {size.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {field.state.meta.errors.length > 0 && (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    )}
                  </Field>
                )}
              </form.Field>
            </div>
          </div>
        </div>

        <div className="bottom-0 mt-auto flex w-full gap-2 px-2 pt-4">
          <Button
            render={<Link preload="intent" to="/app" />}
            variant="outline"
          >
            Cancel
          </Button>

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
                Create new store
              </Button>
            )}
          </form.Subscribe>
        </div>
      </form>
      <UnsavedChangesDialog
        onCancel={reset}
        onDiscard={proceed}
        open={isBlocked}
      />
    </>
  );
};

export default CreateOrganizationForm;
