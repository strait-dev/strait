import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { PhoneInput } from "@strait/ui/components/phone-input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Textarea } from "@strait/ui/components/textarea";
import { useCallback, useId, useRef } from "react";
import { CountryDropdown } from "@/components/common/country-dropdown";
import { useOnboardingAnalytics } from "@/hooks/analytics/use-onboarding-analytics";
import { countries } from "@/utils/data";
import {
  annualRevenues,
  businessTypes,
  companySizes,
} from "../data/company-sizes";
import { industries } from "../data/industries";
import type { OnboardingStepProps } from "../types";

export const CompanyInfoStep = ({ form, isLoading }: OnboardingStepProps) => {
  const { trackCompanyInfoFieldFilled } = useOnboardingAnalytics();
  // Track which fields have been filled to avoid duplicate events
  const trackedFieldsRef = useRef<Set<string>>(new Set());
  const industrySelectId = useId();
  const sizeSelectId = useId();
  const businessTypeSelectId = useId();
  const revenueSelectId = useId();

  const handleFieldFilled = useCallback(
    (fieldName: string) => {
      if (!trackedFieldsRef.current.has(fieldName)) {
        trackedFieldsRef.current.add(fieldName);
        trackCompanyInfoFieldFilled(fieldName);
      }
    },
    [trackCompanyInfoFieldFilled]
  );

  const handleCountryChange = useCallback(
    (countryCode: string | undefined) => {
      if (!countryCode) {
        return;
      }

      const matchingCountry = countries.find(
        (country) => country.iso === countryCode
      );
      if (matchingCountry) {
        form.setFieldValue("country", matchingCountry.value);
      }
    },
    [form]
  );

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h2 className="font-semibold text-secondary-foreground text-xl tracking-tight">
          Tell us about your company
        </h2>
        <p className="whitespace-normal text-muted-foreground text-sm">
          This helps us personalize your experience
        </p>
      </div>

      <div className="space-y-5">
        {/* Company Name and Phone - Row 1 */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <form.Field name="companyName">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={field.name}>Name</FieldLabel>
                <Input
                  id={field.name}
                  onBlur={(e) => {
                    field.handleBlur();
                    if (e.target.value) {
                      handleFieldFilled("companyName");
                    }
                  }}
                  onChange={(e) => field.handleChange(e.target.value)}
                  placeholder="Enter company name"
                  value={field.state.value}
                />
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>

          <form.Field name="companyPhone">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={field.name}>Phone (optional)</FieldLabel>
                <PhoneInput
                  defaultCountry="US"
                  id={field.name}
                  onBlur={field.handleBlur}
                  onChange={(value) => {
                    field.handleChange(value);
                    if (value) {
                      handleFieldFilled("companyPhone");
                    }
                  }}
                  onCountryChange={handleCountryChange}
                  placeholder="Enter phone number"
                  value={field.state.value ?? ""}
                />
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>
        </div>

        {/* Industry and Company Size - Row 2 */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <form.Field name="industry">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={industrySelectId}>Industry</FieldLabel>
                <Select
                  onValueChange={(value) => {
                    if (value) {
                      field.handleChange(value);
                      handleFieldFilled("industry");
                    }
                  }}
                  value={field.state.value}
                >
                  <SelectTrigger id={industrySelectId}>
                    <SelectValue placeholder="Select your industry" />
                  </SelectTrigger>
                  <SelectContent>
                    {industries.map((industry) => (
                      <SelectItem key={industry.value} value={industry.value}>
                        {industry.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>

          <form.Field name="companySize">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={sizeSelectId}>Size</FieldLabel>
                <Select
                  onValueChange={(value) => {
                    if (value) {
                      field.handleChange(value);
                      handleFieldFilled("companySize");
                    }
                  }}
                  value={field.state.value}
                >
                  <SelectTrigger id={sizeSelectId}>
                    <SelectValue placeholder="Select size" />
                  </SelectTrigger>
                  <SelectContent>
                    {companySizes.map((size) => (
                      <SelectItem key={size.value} value={size.value}>
                        {size.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>
        </div>

        {/* Business Type and Annual Revenue - Row 3 */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <form.Field name="businessType">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={businessTypeSelectId}>
                  Business Type
                </FieldLabel>
                <Select
                  onValueChange={(value) => {
                    if (value) {
                      field.handleChange(value);
                      handleFieldFilled("businessType");
                    }
                  }}
                  value={field.state.value}
                >
                  <SelectTrigger id={businessTypeSelectId}>
                    <SelectValue placeholder="Select business type" />
                  </SelectTrigger>
                  <SelectContent>
                    {businessTypes.map((type) => (
                      <SelectItem key={type.value} value={type.value}>
                        {type.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>

          <form.Field name="annualRevenue">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={revenueSelectId}>
                  Annual Revenue (optional)
                </FieldLabel>
                <Select
                  onValueChange={(value) => {
                    field.handleChange(value as typeof field.state.value);
                    handleFieldFilled("annualRevenue");
                  }}
                  value={field.state.value ?? undefined}
                >
                  <SelectTrigger id={revenueSelectId}>
                    <SelectValue placeholder="Select revenue range" />
                  </SelectTrigger>
                  <SelectContent>
                    {annualRevenues.map((revenue) => (
                      <SelectItem key={revenue.value} value={revenue.value}>
                        {revenue.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>
        </div>

        {/* Country and Website - Row 4 */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <form.Field name="country">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={field.name}>Country</FieldLabel>
                <CountryDropdown
                  disabled={isLoading}
                  onValueChange={(value) => {
                    field.handleChange(value);
                    handleFieldFilled("country");
                  }}
                  placeholder="Select your country"
                  value={field.state.value}
                />
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>

          <form.Field name="website">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={field.name}>Website (optional)</FieldLabel>
                <Input
                  id={field.name}
                  onBlur={(e) => {
                    field.handleBlur();
                    if (e.target.value) {
                      handleFieldFilled("website");
                    }
                  }}
                  onChange={(e) => field.handleChange(e.target.value)}
                  placeholder="https://example.com"
                  type="url"
                  value={field.state.value ?? ""}
                />
                {field.state.meta.errors.length > 0 && (
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                )}
              </Field>
            )}
          </form.Field>
        </div>

        {/* Primary Goals - Full Width */}
        <form.Field name="primaryGoals">
          {(field) => (
            <Field>
              <FieldLabel htmlFor={field.name}>
                What are your primary goals? (optional)
              </FieldLabel>
              <Textarea
                className="min-h-20"
                id={field.name}
                onBlur={(e) => {
                  field.handleBlur();
                  if (e.target.value) {
                    handleFieldFilled("primaryGoals");
                  }
                }}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="Tell us about what you hope to achieve with Strait..."
                value={field.state.value ?? ""}
              />
              {field.state.meta.errors.length > 0 && (
                <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
              )}
            </Field>
          )}
        </form.Field>
      </div>
    </div>
  );
};
