import {
  CardCheckboxGroup,
  CardCheckboxItem,
} from "@strait/ui/components/card-checkbox";
import { FieldError } from "@strait/ui/components/field";
import { useCallback } from "react";
import { useOnboardingAnalytics } from "@/hooks/analytics/use-onboarding-analytics";
import { formatFieldErrors } from "@/lib/form-errors";
import { useCaseOptions } from "../data/business-needs-options";
import type { OnboardingStepProps } from "../types";

const BusinessNeedsStep = ({ form }: OnboardingStepProps) => {
  const { trackBusinessNeedSelected, trackBusinessNeedDeselected } =
    useOnboardingAnalytics();

  const createCheckboxChangeHandler = useCallback(
    (
      currentValue: string[],
      optionId: string,
      handleChange: (value: string[]) => void
    ) =>
      (checked: boolean | "indeterminate") => {
        const newValue = checked
          ? [...(currentValue || []), optionId]
          : currentValue?.filter((value: string) => value !== optionId) || [];

        // Track selection/deselection
        if (checked) {
          trackBusinessNeedSelected(optionId, newValue.length);
        } else {
          trackBusinessNeedDeselected(optionId, newValue.length);
        }

        handleChange(newValue);
      },
    [trackBusinessNeedSelected, trackBusinessNeedDeselected]
  );

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h2 className="text-balance font-normal text-secondary-foreground text-xl tracking-tight">
          What will you use Strait for?
        </h2>
        <p className="whitespace-normal text-pretty text-muted-foreground text-sm">
          Select your primary use cases and we'll tailor your experience.
        </p>
      </div>

      <form.Field name="useCases">
        {(field) => (
          <div>
            <CardCheckboxGroup>
              {useCaseOptions.map((option) => (
                <CardCheckboxItem
                  checked={field.state.value?.includes(option.id)}
                  id={option.id}
                  key={option.id}
                  label={option.label}
                  onCheckedChange={createCheckboxChangeHandler(
                    field.state.value,
                    option.id,
                    field.handleChange
                  )}
                />
              ))}
            </CardCheckboxGroup>
            {field.state.meta.errors.length > 0 && (
              <FieldError>
                {formatFieldErrors(field.state.meta.errors)}
              </FieldError>
            )}
          </div>
        )}
      </form.Field>
    </div>
  );
};

export default BusinessNeedsStep;
