import {
  CardCheckboxGroup,
  CardCheckboxItem,
} from "@strait/ui/components/card-checkbox.tsx";
import { FieldError } from "@strait/ui/components/field.tsx";
import { useCallback } from "react";
import { useOnboardingAnalytics } from "@/hooks/analytics/use-onboarding-analytics.ts";
import { businessNeedsOptions } from "../data/business-needs-options.ts";
import type { OnboardingStepProps } from "../types.ts";

export const BusinessNeedsStep = ({ form }: OnboardingStepProps) => {
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
        <h2 className="font-semibold text-secondary-foreground text-xl tracking-tight">
          What brings you to Strait?
        </h2>
        <p className="whitespace-normal text-muted-foreground text-sm">
          Select your business priorities below and we'll help you achieve them
          in Strait.
        </p>
      </div>

      <form.Field name="businessNeeds">
        {(field) => (
          <div>
            <CardCheckboxGroup>
              {businessNeedsOptions.map((option) => (
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
              <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
            )}
          </div>
        )}
      </form.Field>
    </div>
  );
};
