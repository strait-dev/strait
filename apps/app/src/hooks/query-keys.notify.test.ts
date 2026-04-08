import { describe, expect, it } from "vitest";
import { queryKeys } from "./query-keys";

describe("notify query keys", () => {
  it("builds deliveries key", () => {
    expect(
      queryKeys.notify.deliveries({ status: "failed", limit: 20 }).queryKey
    ).toEqual([
      "notify",
      "deliveries",
      { search: { status: "failed", limit: 20 } },
    ]);
  });

  it("builds provider and policy keys", () => {
    expect(queryKeys.notify.providers("email").queryKey).toEqual([
      "notify",
      "providers",
      { channel: "email" },
    ]);

    expect(
      queryKeys.notify.policiesList({ scope_type: "project" }).queryKey
    ).toEqual([
      "notify",
      "policiesList",
      { search: { scope_type: "project" } },
    ]);
  });

  it("builds escalation detail key", () => {
    expect(queryKeys.notify.escalationDetail("step-run-id").queryKey).toEqual([
      "notify",
      "escalationDetail",
      "step-run-id",
    ]);
  });
});
