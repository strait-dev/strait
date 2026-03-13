import type { FormOptions, ReactFormExtendedApi } from "@tanstack/react-form";
import type { OnboardingFormData } from "@/lib/schema.ts";

type InferFormReturn<TOpts> =
  TOpts extends FormOptions<
    infer D,
    infer M,
    infer C,
    infer CA,
    infer B,
    infer BA,
    infer S,
    infer SA,
    infer Dy,
    infer DA,
    infer Sv,
    infer SM
  >
    ? ReactFormExtendedApi<D, M, C, CA, B, BA, S, SA, Dy, DA, Sv, SM>
    : never;

type OnboardingFormOptions = FormOptions<
  OnboardingFormData,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  undefined,
  unknown
>;

export type OnboardingForm = InferFormReturn<OnboardingFormOptions>;

export type OnboardingStepProps = {
  form: OnboardingForm;
  isLoading?: boolean;
};
