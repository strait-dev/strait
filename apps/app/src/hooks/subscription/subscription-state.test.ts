import { describe, expect, it } from "vitest";
import {
  deriveSubscriptionState,
  type NormalizedSubscription,
  nextPlanFor,
  normalizePlanSlug,
  type PlanSlug,
} from "./subscription-state";

const makeSub = (
  overrides: Partial<NormalizedSubscription> = {}
): NormalizedSubscription => ({
  id: "sub_123",
  status: "active",
  productId: "price_starter_monthly",
  priceId: "price_starter_monthly",
  lookupKey: "strait_starter_monthly",
  currentPeriodEnd: new Date("2026-05-01T00:00:00Z"),
  cancelAtPeriodEnd: false,
  recurringInterval: "month",
  trialEnd: null,
  ...overrides,
});

// Fixed timestamp for deterministic trial day calculations.
const NOW = new Date("2026-04-01T12:00:00Z").getTime();

// ---------------------------------------------------------------------------
// normalizePlanSlug
// ---------------------------------------------------------------------------
describe("normalizePlanSlug", () => {
  it.each<[string, PlanSlug]>([
    ["free", "free"],
    ["starter", "starter"],
    ["pro", "pro"],
    ["scale", "scale"],
    ["business", "business"],
    ["enterprise", "enterprise"],
  ])('accepts "%s"', (input, expected) => {
    expect(normalizePlanSlug(input)).toBe(expected);
  });

  it.each([
    "trial",
    "premium",
    "basic",
    "",
    "FREE",
    "Pro",
  ])('rejects unknown value "%s"', (input) => {
    expect(normalizePlanSlug(input)).toBeNull();
  });

  it("returns null for null", () => {
    expect(normalizePlanSlug(null)).toBeNull();
  });

  it("returns null for undefined", () => {
    expect(normalizePlanSlug(undefined)).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// nextPlanFor
// ---------------------------------------------------------------------------
describe("nextPlanFor", () => {
  it("free -> starter", () => {
    expect(nextPlanFor("free")).toEqual({ plan: "starter", name: "Starter" });
  });

  it("starter -> pro", () => {
    expect(nextPlanFor("starter")).toEqual({ plan: "pro", name: "Pro" });
  });

  it("pro -> scale", () => {
    expect(nextPlanFor("pro")).toEqual({ plan: "scale", name: "Scale" });
  });

  it("scale -> business", () => {
    expect(nextPlanFor("scale")).toEqual({
      plan: "business",
      name: "Business",
    });
  });

  it("business -> enterprise", () => {
    expect(nextPlanFor("business")).toEqual({
      plan: "enterprise",
      name: "Enterprise",
    });
  });

  it("enterprise -> null (top tier)", () => {
    expect(nextPlanFor("enterprise")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState -- plan resolution
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — plan resolution", () => {
  it("prefers Stripe price mapping over backend fallback", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub(),
      planFromProduct: "starter",
      backendPlan: "pro",
    });
    expect(state.plan).toBe("starter");
    expect(state.planSlug).toBe("starter");
  });

  it("falls back to backend plan when Stripe mapping is null", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub(),
      planFromProduct: null,
      backendPlan: "pro",
    });
    expect(state.plan).toBe("pro");
  });

  it("defaults to free when both mappings are null", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: null,
    });
    expect(state.plan).toBe("free");
  });

  it("plan and planSlug are always the same value", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub(),
      planFromProduct: "scale",
      backendPlan: null,
    });
    expect(state.plan).toBe(state.planSlug);
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState -- active statuses
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — active statuses", () => {
  it.each([
    "active",
    "trialing",
    "past_due",
    "incomplete",
    "unpaid",
  ] as const)('"%s" is considered active', (status) => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.hasActiveSubscription).toBe(true);
  });

  it.each([
    "canceled",
    "incomplete_expired",
    "paused",
  ] as const)('"%s" is not considered active', (status) => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.hasActiveSubscription).toBe(false);
  });

  it('"none" (no subscription) is not active', () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: null,
    });
    expect(state.hasActiveSubscription).toBe(false);
    expect(state.status).toBe("none");
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — isActive (active AND not canceled)
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — isActive", () => {
  it("active subscription is active", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "active" }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.isActive).toBe(true);
  });

  it("canceled subscription is not active even though hasActiveSubscription may differ", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "canceled" }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.isActive).toBe(false);
    expect(state.isCanceled).toBe(true);
  });

  it("trialing subscription is active", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing" }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.isActive).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — attention / payment
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — needsAttention", () => {
  it.each([
    "past_due",
    "incomplete",
    "unpaid",
  ] as const)('"%s" needs attention', (status) => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.needsAttention).toBe(true);
    expect(state.hasPendingPayment).toBe(true);
  });

  it("active does not need attention", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "active" }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.needsAttention).toBe(false);
    expect(state.hasPendingPayment).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — trial
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — trial", () => {
  it("detects trialing status with trial info", () => {
    const trialEnd = new Date("2026-04-15T00:00:00Z");
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing", trialEnd }),
      planFromProduct: "starter",
      backendPlan: null,
      now: NOW,
    });

    expect(state.isTrialing).toBe(true);
    expect(state.trialInfo).toEqual({ trialEnd });
    expect(state.trialDaysLeft).toBe(14); // Apr 1 -> Apr 15
  });

  it("trialDaysLeft is 0 when trial has expired", () => {
    const trialEnd = new Date("2026-03-30T00:00:00Z"); // before NOW
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing", trialEnd }),
      planFromProduct: "starter",
      backendPlan: null,
      now: NOW,
    });
    expect(state.trialDaysLeft).toBe(0);
  });

  it("trialDaysLeft is null when not trialing", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "active" }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.trialDaysLeft).toBeNull();
    expect(state.trialInfo).toBeNull();
  });

  it("trialDaysLeft is null when trialing but no trialEnd date", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing", trialEnd: null }),
      planFromProduct: "starter",
      backendPlan: null,
      now: NOW,
    });
    expect(state.isTrialing).toBe(true);
    expect(state.trialDaysLeft).toBeNull();
  });

  it("rounds up partial days", () => {
    // 1.5 days from now -> should be 2
    const trialEnd = new Date(NOW + 1.5 * 24 * 60 * 60 * 1000);
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing", trialEnd }),
      planFromProduct: "starter",
      backendPlan: null,
      now: NOW,
    });
    expect(state.trialDaysLeft).toBe(2);
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — shouldShowUpgrade
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — shouldShowUpgrade", () => {
  it("shows upgrade for free (no subscription)", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: null,
    });
    expect(state.shouldShowUpgrade).toBe(true);
  });

  it("shows upgrade for trialing users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "trialing" }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.shouldShowUpgrade).toBe(true);
  });

  it("shows upgrade for past_due users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "past_due" }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.shouldShowUpgrade).toBe(true);
  });

  it("shows upgrade for canceled users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "canceled" }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.shouldShowUpgrade).toBe(true);
  });

  it("does not show upgrade for active paid users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "active" }),
      planFromProduct: "pro",
      backendPlan: null,
    });
    expect(state.shouldShowUpgrade).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — nextPlan
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — nextPlan", () => {
  it("suggests starter for free users", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: "free",
    });
    expect(state.nextPlan).toEqual({ plan: "starter", name: "Starter" });
  });

  it("suggests pro for starter users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub(),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.nextPlan).toEqual({ plan: "pro", name: "Pro" });
  });

  it("returns null for enterprise users", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub(),
      planFromProduct: "enterprise",
      backendPlan: null,
    });
    expect(state.nextPlan).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// deriveSubscriptionState — subscription object passthrough
// ---------------------------------------------------------------------------
describe("deriveSubscriptionState — subscription passthrough", () => {
  it("includes subscription details when present", () => {
    const sub = makeSub({
      id: "sub_abc",
      priceId: "price_pro",
      recurringInterval: "year",
    });
    const state = deriveSubscriptionState({
      subscription: sub,
      planFromProduct: "pro",
      backendPlan: null,
    });

    expect(state.subscription).not.toBeNull();
    expect(state.subscription?.id).toBe("sub_abc");
    expect(state.subscription?.priceId).toBe("price_pro");
    expect(state.subscription?.recurringInterval).toBe("year");
  });

  it("subscription is null when no subscription exists", () => {
    const state = deriveSubscriptionState({
      subscription: null,
      planFromProduct: null,
      backendPlan: null,
    });
    expect(state.subscription).toBeNull();
  });

  it("normalizes cancelled to canceled in subscription object", () => {
    const state = deriveSubscriptionState({
      subscription: makeSub({ status: "cancelled" as string }),
      planFromProduct: "starter",
      backendPlan: null,
    });
    expect(state.status).toBe("canceled");
    expect(state.subscription?.status).toBe("canceled");
  });
});
