import { beforeEach, describe, expect, it, vi } from "vitest";

const mockQuery = vi.fn();
vi.mock("@/lib/auth.server", () => ({
  getAuthPool: () => ({ query: mockQuery }),
}));

import { requireOrgAccess, requireProjectAccess } from "../require-access";

describe("requireOrgAccess", () => {
  beforeEach(() => {
    mockQuery.mockReset();
  });

  it("passes when member row exists", async () => {
    mockQuery.mockResolvedValue({ rowCount: 1 });
    await expect(requireOrgAccess("user-1", "org-1")).resolves.toBeUndefined();
  });

  it("throws Forbidden when user is not a member", async () => {
    mockQuery.mockResolvedValue({ rowCount: 0 });
    await expect(requireOrgAccess("user-1", "org-1")).rejects.toThrow(
      "Forbidden"
    );
  });

  it("throws Forbidden when organizationId is empty", async () => {
    await expect(requireOrgAccess("user-1", "")).rejects.toThrow("Forbidden");
  });

  it("throws Forbidden when userId is empty", async () => {
    await expect(requireOrgAccess("", "org-1")).rejects.toThrow("Forbidden");
  });

  it("handles database errors gracefully", async () => {
    mockQuery.mockRejectedValue(new Error("connection refused"));
    await expect(requireOrgAccess("user-1", "org-1")).rejects.toThrow(
      "connection refused"
    );
  });
});

describe("requireProjectAccess", () => {
  beforeEach(() => {
    mockQuery.mockReset();
  });

  it("passes when project belongs to user's org", async () => {
    // First call: member check, second call: project check
    mockQuery
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rowCount: 1 });
    await expect(
      requireProjectAccess("user-1", "proj-1", "org-1")
    ).resolves.toBeUndefined();
  });

  it("throws when project belongs to different org", async () => {
    // Member check passes, project check fails
    mockQuery
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rowCount: 0 });
    await expect(
      requireProjectAccess("user-1", "proj-1", "org-1")
    ).rejects.toThrow("Forbidden");
  });

  it("throws when user is not a member of the org", async () => {
    mockQuery.mockResolvedValueOnce({ rowCount: 0 });
    await expect(
      requireProjectAccess("user-1", "proj-1", "org-1")
    ).rejects.toThrow("Forbidden");
  });

  it("throws when project does not exist", async () => {
    mockQuery
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rowCount: 0 });
    await expect(
      requireProjectAccess("user-1", "proj-nonexistent", "org-1")
    ).rejects.toThrow("Forbidden");
  });

  it("throws when activeOrganizationId is empty", async () => {
    await expect(requireProjectAccess("user-1", "proj-1", "")).rejects.toThrow(
      "Forbidden"
    );
  });

  it("throws when projectId is empty", async () => {
    await expect(requireProjectAccess("user-1", "", "org-1")).rejects.toThrow(
      "Forbidden"
    );
  });

  it("throws when activeOrganizationId is undefined", async () => {
    await expect(
      requireProjectAccess("user-1", "proj-1", undefined)
    ).rejects.toThrow("Forbidden");
  });

  it("handles database errors gracefully", async () => {
    mockQuery.mockRejectedValue(new Error("connection refused"));
    await expect(
      requireProjectAccess("user-1", "proj-1", "org-1")
    ).rejects.toThrow("connection refused");
  });
});
