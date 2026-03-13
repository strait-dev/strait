import { create } from "zustand";

type OnboardingStore = {
  currentStep: number;
  totalSteps: number;
  setCurrentStep: (step: number | ((prev: number) => number)) => void;
  reset: () => void;
};

export const useOnboardingStore = create<OnboardingStore>((set) => ({
  currentStep: 1,
  totalSteps: 2, // Now only 2 steps: Business Needs, Company Info
  setCurrentStep: (step) =>
    set((state) => ({
      currentStep: typeof step === "function" ? step(state.currentStep) : step,
    })),
  reset: () =>
    set({
      currentStep: 1,
    }),
}));
