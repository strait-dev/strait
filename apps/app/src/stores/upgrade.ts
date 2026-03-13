import { create } from "zustand";

type PlanSlug = "starter" | "growth" | "professional" | "enterprise";
type BillingInterval = "monthly" | "yearly";

type UpgradeStore = {
  selectedPlan: PlanSlug;
  billingInterval: BillingInterval;
  setSelectedPlan: (plan: PlanSlug) => void;
  setBillingInterval: (interval: BillingInterval) => void;
  reset: () => void;
};

export const useUpgradeStore = create<UpgradeStore>((set) => ({
  selectedPlan: "growth", // Default to Growth plan (Most Popular)
  billingInterval: "monthly", // Default to monthly
  setSelectedPlan: (plan) => set({ selectedPlan: plan }),
  setBillingInterval: (interval) => set({ billingInterval: interval }),
  reset: () =>
    set({
      selectedPlan: "growth",
      billingInterval: "monthly",
    }),
}));
