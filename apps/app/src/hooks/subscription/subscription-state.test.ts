import { describe, expect, it } from "vitest";
import {
  deriveSubscriptionState,
  type NormalizedSubscription,
  normalizePlanSlug,
} from "./subscription-state";

const baseSubscription: NormalizedSubscription = {
  id: "sub_123",
  status: "active",
  productId: "prod_starter",
  priceId: "price_starter",
  currentPeriodEnd: new Date("2026-04-01T00:00:00Z"),
  cancelAtPeriodEnd: false,
  recurringInterval: "month",
  trialEnd: null,
};

describe("normalizePlanSlug", () => {
  it("accepts known plan slugs", () => {
    expect(normalizePlanSlug("free")).toBe("free");
    expect(normalizePlanSlug("starter")).toBe("starter");
    expect(normalizePlanSlug("pro")).toBe("pro");
    expect(normalizePlanSlug("scale")).toBe("scale");
    expect(normalizePlanSlug("enterprise")).toBe("enterprise");
  });

  it("rejects unknown plan values", () => {
    expect(normalizePlanSlug("trial")).toBeNull();
    expect(normalizePlanSlug("")).toBeNull();
    expect(normalizePlanSlug(undefined)).toBeNull();
  });
});

describe("deriveSubscriptionState", () => {
  it("uses free when backend reports free and Polar has no subscription", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: "free",
    });

    expect(state.planSlug).toBe("free");
    expect(state.plan).toBe("free");
    expect(state.nextPlan).toEqual({ plan: "starter", name: "Starter" });
  });

  it("uses backend plan when Polar has no subscription", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: "pro",
    });

    expect(state.planSlug).toBe("pro");
    expect(state.plan).toBe("pro");
    expect(state.nextPlan).toEqual({ plan: "scale", name: "Scale" });
  });

  it("defaults to free when both Polar and backend plans are missing", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: null,
    });

    expect(state.planSlug).toBe("free");
    expect(state.plan).toBe("free");
    expect(state.nextPlan).toEqual({ plan: "starter", name: "Starter" });
  });

  it("prefers the Polar-mapped plan over the backend fallback", () => {
    const state = deriveSubscriptionState({
      subscription: baseSubscription,
      planFromProduct: "starter",
      backendPlan: "pro",
    });

    expect(state.planSlug).toBe("starter");
    expect(state.plan).toBe("starter");
    expect(state.subscription?.status).toBe("active");
    expect(state.nextPlan).toEqual({ plan: "pro", name: "Pro" });
  });

  it("returns no next plan for enterprise", () => {
    const state = deriveSubscriptionState({
      subscription: {
        ...baseSubscription,
        status: "active",
        productId: "prod_enterprise",
      },
      planFromProduct: "enterprise",
      backendPlan: null,
    });

    expect(state.planSlug).toBe("enterprise");
    expect(state.plan).toBe("enterprise");
    expect(state.nextPlan).toBeNull();
  });
});
