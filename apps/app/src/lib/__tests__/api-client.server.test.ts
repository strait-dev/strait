import { afterEach, describe, expect, it, vi } from "vitest";
import { apiPath, apiPathSegment, apiRequest } from "@/lib/api-client.server";

vi.mock("@tanstack/react-start/server", () => ({
  getRequestHeaders: () => new Headers(),
}));

vi.mock("@/lib/auth.server", () => ({
  getAuth: async () => ({
    api: {
      getSession: async () => ({
        user: { id: "user-1", activeProjectId: "project-1" },
        session: { activeOrganizationId: "org-1" },
      }),
    },
  }),
}));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("apiPathSegment", () => {
  it("encodes safe path parameters", () => {
    expect(apiPath`/v1/jobs/${"job:123"}/runs`).toBe("/v1/jobs/job%3A123/runs");
  });

  it("rejects route-control characters", () => {
    expect(() => apiPathSegment("../api-keys")).toThrow(
      "route-control characters"
    );
    expect(() => apiPathSegment("run/123")).toThrow("route-control characters");
    expect(() => apiPathSegment("run?x=1")).toThrow("route-control characters");
    expect(() => apiPathSegment("run#frag")).toThrow(
      "route-control characters"
    );
  });

  it("rejects encoded traversal and separator bytes", () => {
    expect(() => apiPathSegment("run%2f123")).toThrow(
      "encoded route-control bytes"
    );
    expect(() => apiPathSegment("%2e%2e")).toThrow(
      "encoded route-control bytes"
    );
  });
});

describe("apiRequest path validation", () => {
  it("rejects raw query and fragment syntax before fetching", async () => {
    vi.stubEnv("INTERNAL_SECRET", "test-secret");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    await expect(apiRequest("/v1/runs/abc?x=1")).rejects.toThrow(
      "query or fragment"
    );
    await expect(apiRequest("/v1/runs/abc#frag")).rejects.toThrow(
      "query or fragment"
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("rejects dot segments and protocol-relative paths before fetching", async () => {
    vi.stubEnv("INTERNAL_SECRET", "test-secret");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    await expect(apiRequest("/v1/runs/../api-keys")).rejects.toThrow(
      "dot segments"
    );
    await expect(apiRequest("//example.test/v1/runs")).rejects.toThrow(
      "protocol-relative"
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("sends active project ID but never falls back to active organization ID", async () => {
    vi.stubEnv("INTERNAL_SECRET", "test-secret");
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ ok: true }),
    });
    vi.stubGlobal("fetch", fetchSpy);

    await apiRequest("/v1/jobs");

    expect(fetchSpy).toHaveBeenCalledWith(
      "http://localhost:8080/v1/jobs",
      expect.objectContaining({
        headers: expect.objectContaining({
          "X-Internal-Secret": "test-secret",
          "X-Project-Id": "project-1",
        }),
      })
    );
  });

  it("can override the project ID for target-project operations", async () => {
    vi.stubEnv("INTERNAL_SECRET", "test-secret");
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });
    vi.stubGlobal("fetch", fetchSpy);

    await apiRequest("/v1/projects/project-2", {
      method: "DELETE",
      projectId: "project-2",
    });

    expect(fetchSpy).toHaveBeenCalledWith(
      "http://localhost:8080/v1/projects/project-2",
      expect.objectContaining({
        headers: expect.objectContaining({
          "X-Project-Id": "project-2",
        }),
      })
    );
  });
});
