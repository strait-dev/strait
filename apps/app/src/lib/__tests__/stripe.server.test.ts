import { Effect } from "effect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  findCustomerByOrg,
  findCustomerByOrgEffect,
  findOrCreateCustomerForOrgEffect,
  StripeIntegrationError,
} from "@/lib/stripe.server";

const stripeMocks = vi.hoisted(() => ({
  customersList: vi.fn(),
  customersCreate: vi.fn(),
}));

vi.mock("stripe", () => ({
  default: vi.fn(function MockStripe() {
    return {
      customers: {
        list: stripeMocks.customersList,
        create: stripeMocks.customersCreate,
      },
    };
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubEnv("STRIPE_SECRET_KEY", "sk_test_123");
});

describe("findCustomerByOrgEffect", () => {
  it("returns the customer matching the organization metadata", async () => {
    stripeMocks.customersList.mockResolvedValue({
      data: [
        { id: "cus_other", metadata: { org_id: "org-other" } },
        { id: "cus_match", metadata: { org_id: "org-1" } },
      ],
    });

    const result = await Effect.runPromise(
      findCustomerByOrgEffect("owner@example.com", "org-1")
    );

    expect(result).toBe("cus_match");
    expect(stripeMocks.customersList).toHaveBeenCalledWith({
      email: "owner@example.com",
      limit: 100,
    });
  });

  it("returns null when no listed customer belongs to the organization", async () => {
    stripeMocks.customersList.mockResolvedValue({
      data: [{ id: "cus_other", metadata: { org_id: "org-other" } }],
    });

    await expect(
      Effect.runPromise(findCustomerByOrgEffect("owner@example.com", "org-1"))
    ).resolves.toBeNull();
  });

  it("returns a typed failure when Stripe lookup fails", async () => {
    stripeMocks.customersList.mockRejectedValue(new Error("stripe down"));

    const result = await Effect.runPromiseExit(
      findCustomerByOrgEffect("owner@example.com", "org-1")
    );

    expect(result._tag).toBe("Failure");
    if (result._tag === "Failure") {
      const cause = result.cause as {
        _tag: string;
        error: StripeIntegrationError;
      };
      expect(cause.error).toBeInstanceOf(StripeIntegrationError);
      expect(cause.error.operation).toBe("customer_lookup");
      expect(cause.error.orgId).toBe("org-1");
      expect(cause.error.email).toBe("owner@example.com");
    }
  });

  it("preserves legacy Promise failures for existing callers", async () => {
    stripeMocks.customersList.mockRejectedValue(new Error("stripe down"));

    await expect(
      findCustomerByOrg("owner@example.com", "org-1")
    ).rejects.toThrow("Stripe customer lookup failed");
  });
});

describe("findOrCreateCustomerForOrgEffect", () => {
  it("reuses an existing organization customer", async () => {
    stripeMocks.customersList.mockResolvedValue({
      data: [{ id: "cus_existing", metadata: { org_id: "org-1" } }],
    });

    const result = await Effect.runPromise(
      findOrCreateCustomerForOrgEffect({
        email: "owner@example.com",
        orgId: "org-1",
        userId: "user-1",
        name: "Owner",
      })
    );

    expect(result).toBe("cus_existing");
    expect(stripeMocks.customersCreate).not.toHaveBeenCalled();
  });

  it("creates a customer with org and user metadata when none exists", async () => {
    stripeMocks.customersList.mockResolvedValue({ data: [] });
    stripeMocks.customersCreate.mockResolvedValue({ id: "cus_created" });

    const result = await Effect.runPromise(
      findOrCreateCustomerForOrgEffect({
        email: "owner@example.com",
        orgId: "org-1",
        userId: "user-1",
        name: "Owner",
      })
    );

    expect(result).toBe("cus_created");
    expect(stripeMocks.customersCreate).toHaveBeenCalledWith({
      email: "owner@example.com",
      name: "Owner",
      metadata: {
        org_id: "org-1",
        user_id: "user-1",
      },
    });
  });

  it("omits optional user metadata when userId is absent", async () => {
    stripeMocks.customersList.mockResolvedValue({ data: [] });
    stripeMocks.customersCreate.mockResolvedValue({ id: "cus_created" });

    await Effect.runPromise(
      findOrCreateCustomerForOrgEffect({
        email: "owner@example.com",
        orgId: "org-1",
        name: null,
      })
    );

    expect(stripeMocks.customersCreate).toHaveBeenCalledWith({
      email: "owner@example.com",
      name: undefined,
      metadata: {
        org_id: "org-1",
      },
    });
  });
});
