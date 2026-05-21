import { beforeEach, describe, expect, it, vi } from "vitest";

const { mockApiRequest, mockRequireActiveProjectAccess } = vi.hoisted(() => ({
  mockApiRequest: vi.fn(),
  mockRequireActiveProjectAccess: vi.fn(),
}));

vi.mock("@/lib/api-client.server", () => ({
  apiPath: (
    strings: TemplateStringsArray,
    ...segments: Array<string | number>
  ) =>
    strings.reduce(
      (path, part, index) => path + part + (segments[index] ?? ""),
      ""
    ),
  apiRequest: mockApiRequest,
}));

vi.mock("@/middlewares/require-access", () => ({
  requireActiveProjectAccess: mockRequireActiveProjectAccess,
}));

import { requireActiveProjectScope } from "../require-project-scope";

describe("requireActiveProjectScope", () => {
  beforeEach(() => {
    mockApiRequest.mockReset();
    mockRequireActiveProjectAccess.mockReset();
    mockRequireActiveProjectAccess.mockResolvedValue("project-1");
  });

  it("allows access when the user's project role includes the required scope", async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        data: [{ user_id: "user-1", role_id: "role-1" }],
        has_more: false,
      })
      .mockResolvedValueOnce({
        role: { permissions: ["jobs:read"] },
        lineage: [],
      });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).resolves.toBe("project-1");
    expect(mockApiRequest).toHaveBeenNthCalledWith(1, "/v1/members", {
      params: { limit: 100 },
      projectId: "project-1",
    });
    expect(mockApiRequest).toHaveBeenNthCalledWith(2, "/v1/roles/role-1", {
      params: { include_lineage: true },
      projectId: "project-1",
    });
  });

  it("allows access when an inherited role includes the required scope", async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        data: [{ user_id: "user-1", role_id: "role-1" }],
        has_more: false,
      })
      .mockResolvedValueOnce({
        role: { permissions: ["jobs:write"] },
        lineage: [{ permissions: ["jobs:read"] }],
      });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).resolves.toBe("project-1");
  });

  it("allows access when the role has wildcard permissions", async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        data: [{ user_id: "user-1", role_id: "role-1" }],
        has_more: false,
      })
      .mockResolvedValueOnce({
        role: { permissions: ["*"] },
        lineage: [],
      });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).resolves.toBe("project-1");
  });

  it("rejects access when the user has no project member role", async () => {
    mockApiRequest.mockResolvedValueOnce({
      data: [{ user_id: "other-user", role_id: "role-1" }],
      has_more: false,
    });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).rejects.toThrow("Forbidden");
    expect(mockApiRequest).toHaveBeenCalledTimes(1);
  });

  it("rejects access when the project role lacks the required scope", async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        data: [{ user_id: "user-1", role_id: "role-1" }],
        has_more: false,
      })
      .mockResolvedValueOnce({
        role: { permissions: ["jobs:write"] },
        lineage: [],
      });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).rejects.toThrow("Forbidden");
  });

  it("continues through member pages until it finds the user's role", async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        data: [{ user_id: "other-user", role_id: "role-1" }],
        has_more: true,
        next_cursor: "cursor-2",
      })
      .mockResolvedValueOnce({
        data: [{ user_id: "user-1", role_id: "role-2" }],
        has_more: false,
      })
      .mockResolvedValueOnce({
        role: { permissions: ["jobs:read"] },
        lineage: [],
      });

    await expect(
      requireActiveProjectScope({ user: { id: "user-1" } }, "jobs:read")
    ).resolves.toBe("project-1");
    expect(mockApiRequest).toHaveBeenNthCalledWith(1, "/v1/members", {
      params: { limit: 100 },
      projectId: "project-1",
    });
    expect(mockApiRequest).toHaveBeenNthCalledWith(2, "/v1/members", {
      params: { limit: 100, cursor: "cursor-2" },
      projectId: "project-1",
    });
    expect(mockApiRequest).toHaveBeenNthCalledWith(3, "/v1/roles/role-2", {
      params: { include_lineage: true },
      projectId: "project-1",
    });
  });
});
