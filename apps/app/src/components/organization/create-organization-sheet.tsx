import {
  Add01Icon,
  Loading03Icon,
  Search01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field.tsx";
import { Input } from "@strait/ui/components/input.tsx";
import { InputWithLoader } from "@strait/ui/components/input-with-loader.tsx";
import { PhoneInput } from "@strait/ui/components/phone-input.tsx";
import { ScrollArea } from "@strait/ui/components/scroll-area.tsx";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select.tsx";
import { Separator } from "@strait/ui/components/separator.tsx";
import {
  type Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet.tsx";
import { Textarea } from "@strait/ui/components/textarea.tsx";
import { toast } from "@strait/ui/components/toast/index.ts";
import { useForm } from "@tanstack/react-form";
import { nanoid } from "nanoid";
import { useId, useMemo, useTransition } from "react";
import { z } from "zod/v4";
import { useCreateOrganization } from "@/hooks/auth/use-organization.ts";
import type { AuthUser } from "@/routes/__root.tsx";
import { ORGANIZATION_SLUG_LENGTH } from "@/utils/constants.ts";
import { countries } from "@/utils/data.ts";
import {
  activities,
  company_size,
  employees_size,
  fiscal_types,
  segments,
} from "@/utils/options.tsx";

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

type Props = React.ComponentPropsWithRef<typeof Sheet> & {
  onClose: () => void;
  user: AuthUser;
};

const CreateOrganizationSheet = ({ onClose, user }: Props) => {
  const statusSelectId = useId();
  const entityTypeSelectId = useId();
  const segmentSelectId = useId();
  const activitySelectId = useId();
  const companySizeSelectId = useId();
  const employeesSizeSelectId = useId();
  const countrySelectId = useId();

  const [isPending] = useTransition();

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
        slug: `${value.name.toLowerCase().replace(/\s+/g, "-")}-${nanoid(ORGANIZATION_SLUG_LENGTH)}`,
      };

      const parsedValues = insertOrganizationSchema.parse(data);

      toast.promise(createOrganization.mutateAsync(parsedValues), {
        loading: "Creating store...",
        success: () => {
          form.reset();

          onClose();
          return "Store created successfully!";
        },
        error:
          "Error creating store. Please try again. If the problem persists, contact support.",
      });
    },
  });

  return (
    <SheetContent className="w-[400px] sm:w-[700px] sm:max-w-[700px]">
      <form
        className="flex h-full flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          form.handleSubmit();
        }}
      >
        <SheetHeader className="px-4">
          <SheetTitle>Create new store</SheetTitle>
          <SheetDescription>
            Here you can create a new store to sell your products. Fill out the
            form below with as much information as possible. You can edit this
            information at any time later.
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-hidden">
          <ScrollArea className="h-[calc(100vh-13rem)] pr-4">
            <div className="px-4 pb-6">
              {/* Information */}
              <div className="flex flex-col gap-4">
                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
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

                  <form.Field name="email">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>Email</FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the store email"
                          type="email"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="status">
                    {(field) => (
                      <Field>
                        <FieldLabel htmlFor={statusSelectId}>Status</FieldLabel>
                        <Select
                          onValueChange={(val) =>
                            field.handleChange(val as typeof field.state.value)
                          }
                          value={field.state.value ?? undefined}
                        >
                          <SelectTrigger id={statusSelectId}>
                            <SelectValue placeholder="Select the status" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="active">Active</SelectItem>
                            <SelectItem value="inactive">Inactive</SelectItem>
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="phone">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Mobile Phone
                        </FieldLabel>
                        <PhoneInput
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(value) => field.handleChange(value)}
                          placeholder="Enter the mobile number"
                          type="tel"
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

                <form.Field name="description">
                  {(field) => (
                    <Field className="w-full">
                      <FieldLabel htmlFor={field.name}>Description</FieldLabel>
                      <Textarea
                        id={field.name}
                        onBlur={field.handleBlur}
                        onChange={(e) => field.handleChange(e.target.value)}
                        placeholder="Enter the store description"
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
                <div className="flex flex-col justify-between gap-2 lg:flex-row">
                  <div className="flex flex-col gap-1">
                    <h3 className="font-semibold text-base text-secondary-foreground tracking-normal">
                      Tax Information
                    </h3>
                    <p className="text-muted-foreground text-sm">
                      Add your store's tax information here, such as Tax ID,
                      Business Registration, etc.
                    </p>
                  </div>
                </div>

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

                  <form.Field name="segment">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={segmentSelectId}>
                          Segment
                        </FieldLabel>
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
                              <SelectItem
                                key={segment.value}
                                value={segment.value}
                              >
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

                  <form.Field name="taxId">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>Tax ID</FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter your tax ID"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="businessName">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Business Name
                        </FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter business name"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="businessRegistration">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Business Registration
                        </FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter business registration"
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

                  <form.Field name="industryCode">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Industry Code
                        </FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter industry code"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
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

                  <form.Field name="employeesSize">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={employeesSizeSelectId}>
                          Number of Employees
                        </FieldLabel>
                        <Select
                          onValueChange={(val) =>
                            field.handleChange(val as typeof field.state.value)
                          }
                          value={field.state.value ?? undefined}
                        >
                          <SelectTrigger id={employeesSizeSelectId}>
                            <SelectValue placeholder="Select the number of employees" />
                          </SelectTrigger>
                          <SelectContent>
                            {employees_size.map((size) => (
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

              <Separator className="mt-6 mb-6" />

              {/* Address */}
              <div className="flex flex-col gap-4">
                <div className="flex flex-col justify-between gap-2 lg:flex-row">
                  <div className="flex flex-col gap-1">
                    <h3 className="font-semibold text-base text-secondary-foreground tracking-normal">
                      Address
                    </h3>
                    <p className="text-muted-foreground text-sm">
                      Add your supplier's address here. This will make it easier
                      to locate you and your suppliers.
                    </p>
                  </div>
                </div>

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="postalCode">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Postal Code
                        </FieldLabel>
                        <InputWithLoader
                          icon={
                            <HugeiconsIcon
                              className="size-4"
                              icon={Search01Icon}
                            />
                          }
                          id={field.name}
                          isLoading={isPending}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the postal code"
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

                  <form.Field name="address">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>Address</FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the address"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="houseNumber">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>Number</FieldLabel>
                        <Input
                          id={field.name}
                          min={0}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the house number"
                          type="number"
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

                  <form.Field name="complement">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>Complement</FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the complement"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="neighborhood">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          Neighborhood
                        </FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter the neighborhood"
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

                  <form.Field name="state">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>
                          State/Province
                        </FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter state or province"
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

                <div className="grid grid-cols-1 items-start gap-6 sm:grid-cols-2">
                  <form.Field name="city">
                    {(field) => (
                      <Field className="w-full">
                        <FieldLabel htmlFor={field.name}>City</FieldLabel>
                        <Input
                          id={field.name}
                          onBlur={field.handleBlur}
                          onChange={(e) => field.handleChange(e.target.value)}
                          placeholder="Enter city"
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

                  <form.Field name="country">
                    {(field) => (
                      <Field>
                        <FieldLabel htmlFor={countrySelectId}>
                          Country
                        </FieldLabel>
                        <Select
                          defaultValue="Brazil"
                          onValueChange={(val) =>
                            field.handleChange(val as typeof field.state.value)
                          }
                          value={field.state.value ?? undefined}
                        >
                          <SelectTrigger disabled={true} id={countrySelectId}>
                            <SelectValue placeholder="Select the country" />
                          </SelectTrigger>
                          <SelectContent>
                            {countries.map((country) => (
                              <SelectItem
                                key={country.value}
                                value={country.value}
                              >
                                {country.label}
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
          </ScrollArea>
        </div>

        <SheetFooter className="bottom-0 mt-auto flex w-full gap-2 px-4 pt-4">
          <SheetClose
            render={
              <Button
                onClick={() => {
                  if (!isPending) {
                    onClose();
                    form.reset();
                  }
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
                  createOrganization.isPending ||
                  isPending
                }
                type="submit"
              >
                {isSubmitting || createOrganization.isPending || isPending ? (
                  <HugeiconsIcon
                    className="size-4 animate-spin"
                    icon={Loading03Icon}
                  />
                ) : (
                  <HugeiconsIcon className="size-4" icon={Add01Icon} />
                )}
                Create new store
              </Button>
            )}
          </form.Subscribe>
        </SheetFooter>
      </form>
    </SheetContent>
  );
};

export default CreateOrganizationSheet;
