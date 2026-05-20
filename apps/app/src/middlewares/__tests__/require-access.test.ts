import { beforeEach, describe, expect, it, vi } from "vitest";

const mockQuery = vi.fn();
vi.mock("@/lib/auth.server", () => ({
  getAuthPool: () => ({ query: mockQuery }),
}));

import {
  getOrganizationRole,
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
  requireOrgAccess,
  requireOrgAdmin,
  requireOrgOwner,
  requireProjectAccess,
  requireProjectAdmin,
} from "../require-access";

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

describe("role-gated access helpers", () => {
  beforeEach(() => {
    mockQuery.mockReset();
  });

  it("returns the strongest role when multiple roles are stored", async () => {
    mockQuery.mockResolvedValue({ rows: [{ role: "admin,owner" }] });

    await expect(getOrganizationRole("user-1", "org-1")).resolves.toBe("owner");
  });

  it("allows admins for admin-gated organization operations", async () => {
    mockQuery.mockResolvedValue({ rows: [{ role: "admin" }] });

    await expect(requireOrgAdmin("user-1", "org-1")).resolves.toBeUndefined();
  });

  it("rejects members for admin-gated organization operations", async () => {
    mockQuery.mockResolvedValue({ rows: [{ role: "member" }] });

    await expect(requireOrgAdmin("user-1", "org-1")).rejects.toThrow(
      "Forbidden"
    );
  });

  it("requires owner for owner-gated operations", async () => {
    mockQuery.mockResolvedValue({ rows: [{ role: "admin" }] });

    await expect(requireOrgOwner("user-1", "org-1")).rejects.toThrow(
      "Forbidden"
    );
  });

  it("checks project membership before project admin role", async () => {
    mockQuery
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rows: [{ role: "owner" }] });

    await expect(
      requireProjectAdmin("user-1", "project-1", "org-1")
    ).resolves.toBeUndefined();
  });

  it("rejects project admin access when the project is outside the org", async () => {
    mockQuery
      .mockResolvedValueOnce({ rowCount: 1 })
      .mockResolvedValueOnce({ rowCount: 0 });

    await expect(
      requireProjectAdmin("user-1", "project-1", "org-1")
    ).rejects.toThrow("Forbidden");
    expect(mockQuery).toHaveBeenCalledTimes(2);
  });

  it("returns the active project after live membership checks", async () => {
    mockQuery.mockResolvedValue({ rowCount: 1 });

    await expect(
      requireActiveProjectAccess({
        user: { id: "user-1" },
        activeOrganizationId: "org-1",
        activeProjectId: "project-1",
      })
    ).resolves.toBe("project-1");
  });

  it("rejects active project admin checks when the session has no project", async () => {
    await expect(
      requireActiveProjectAdmin({
        user: { id: "user-1" },
        activeOrganizationId: "org-1",
      })
    ).rejects.toThrow("Forbidden");
    expect(mockQuery).not.toHaveBeenCalled();
  });
});
