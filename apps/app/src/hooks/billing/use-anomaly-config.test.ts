import { describe, expect, it } from "vitest";
import { queryKeys } from "../query-keys";

describe("anomalyConfigQueryOptions", () => {
  it("returns correct query key", () => {
    const key = queryKeys.billing.anomalyConfig.queryKey;
    expect(key).toEqual(["billing", "anomalyConfig"]);
  });
});
