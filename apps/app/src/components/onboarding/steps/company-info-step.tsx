import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Textarea } from "@strait/ui/components/textarea";
import { useCallback, useId, useRef } from "react";
import { useOnboardingAnalytics } from "@/hooks/analytics/use-onboarding-analytics";
import { formatFieldErrors } from "@/lib/form-errors";
import { environments, teamSizes } from "../data/company-sizes";
import type { OnboardingStepProps } from "../types";

const CompanyInfoStep = ({ form }: OnboardingStepProps) => {
  const { trackCompanyInfoFieldFilled } = useOnboardingAnalytics();
  const trackedFieldsRef = useRef<Set<string>>(new Set());
  const teamSizeSelectId = useId();
  const environmentSelectId = useId();

  const handleFieldFilled = useCallback(
    (fieldName: string) => {
      if (!trackedFieldsRef.current.has(fieldName)) {
        trackedFieldsRef.current.add(fieldName);
        trackCompanyInfoFieldFilled(fieldName);
      }
    },
    [trackCompanyInfoFieldFilled]
  );

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h2 className="text-balance font-normal text-secondary-foreground text-xl tracking-tight">
          Set up your workspace
        </h2>
        <p className="whitespace-normal text-pretty text-muted-foreground text-sm">
          Configure your workspace to get started with Strait.
        </p>
      </div>

      <div className="space-y-5">
        {/* Workspace Name - Row 1 */}
        <form.Field name="workspaceName">
          {(field) => (
            <Field>
              <FieldLabel htmlFor={field.name}>Workspace Name</FieldLabel>
              <Input
                id={field.name}
                onBlur={(e) => {
                  field.handleBlur();
                  if (e.target.value) {
                    handleFieldFilled("workspaceName");
                  }
                }}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="e.g. Acme Engineering"
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

        {/* Team Size + Environment - Row 2 */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <form.Field name="teamSize">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={teamSizeSelectId}>Team Size</FieldLabel>
                <Select
                  onValueChange={(value) => {
                    if (value) {
                      field.handleChange(value);
                      handleFieldFilled("teamSize");
                    }
                  }}
                  value={field.state.value}
                >
                  <SelectTrigger id={teamSizeSelectId}>
                    <SelectValue placeholder="Select team size" />
                  </SelectTrigger>
                  <SelectContent>
                    {teamSizes.map((size) => (
                      <SelectItem key={size.value} value={size.value}>
                        {size.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>
                    {formatFieldErrors(field.state.meta.errors)}
                  </FieldError>
                )}
              </Field>
            )}
          </form.Field>

          <form.Field name="environment">
            {(field) => (
              <Field>
                <FieldLabel htmlFor={environmentSelectId}>
                  Primary Environment
                </FieldLabel>
                <Select
                  onValueChange={(value) => {
                    if (value) {
                      field.handleChange(value);
                      handleFieldFilled("environment");
                    }
                  }}
                  value={field.state.value}
                >
                  <SelectTrigger id={environmentSelectId}>
                    <SelectValue placeholder="Select environment" />
                  </SelectTrigger>
                  <SelectContent>
                    {environments.map((env) => (
                      <SelectItem key={env.value} value={env.value}>
                        {env.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {field.state.meta.errors.length > 0 && (
                  <FieldError>
                    {formatFieldErrors(field.state.meta.errors)}
                  </FieldError>
                )}
              </Field>
            )}
          </form.Field>
        </div>

        {/* Primary Goals - Row 3 */}
        <form.Field name="primaryGoals">
          {(field) => (
            <Field>
              <FieldLabel htmlFor={field.name}>
                What do you want to achieve? (optional)
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
                <FieldError>
                  {formatFieldErrors(field.state.meta.errors)}
                </FieldError>
              )}
            </Field>
          )}
        </form.Field>
      </div>
    </div>
  );
};

export default CompanyInfoStep;
