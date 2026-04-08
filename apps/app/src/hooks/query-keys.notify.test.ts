import { describe, expect, it } from "vitest";
import { queryKeys } from "./query-keys";

describe("notify query keys", () => {
  it("builds deliveries key", () => {
    expect(
      queryKeys.notify.deliveries({
        status: "failed",
        limit: 20,
        cursor: "2026-01-01T00:00:00Z",
      }).queryKey
    ).toEqual([
      "notify",
      "deliveries",
      {
        search: {
          status: "failed",
          limit: 20,
          cursor: "2026-01-01T00:00:00Z",
        },
      },
    ]);
  });

  it("builds subscriber and template list keys", () => {
    expect(
      queryKeys.notify.subscribersList({
        limit: 25,
        cursor: "2026-01-02T00:00:00Z",
      }).queryKey
    ).toEqual([
      "notify",
      "subscribersList",
      { search: { limit: 25, cursor: "2026-01-02T00:00:00Z" } },
    ]);

    expect(
      queryKeys.notify.templatesList({
        limit: 25,
        cursor: "2026-01-03T00:00:00Z",
      }).queryKey
    ).toEqual([
      "notify",
      "templatesList",
      { search: { limit: 25, cursor: "2026-01-03T00:00:00Z" } },
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

  it("builds subscriber preferences key", () => {
    expect(
      queryKeys.notify.subscriberPreferences("subscriber-id").queryKey
    ).toEqual(["notify", "subscriberPreferences", "subscriber-id"]);
  });
});
