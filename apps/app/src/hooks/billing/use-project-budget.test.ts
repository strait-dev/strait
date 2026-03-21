import { describe, expect, it } from "vitest";
import { queryKeys } from "../query-keys";

describe("projectBudgetQueryOptions", () => {
  it("returns correct query key with project ID", () => {
    const key = queryKeys.billing.projectBudget("proj-123").queryKey;
    expect(key).toEqual(["billing", "projectBudget", "proj-123"]);
  });
});
