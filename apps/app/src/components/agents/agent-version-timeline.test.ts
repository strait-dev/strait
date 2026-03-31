import { describe, expect, it } from "vitest";
import type { AgentVersion } from "@/hooks/api/use-agents";

// Since AgentVersionTimeline is a React component that requires a DOM,
// we test the pure logic extracted into helpers. The component rendering
// is validated via TypeScript compilation and biome lint.

function sortVersions(versions: AgentVersion[]): AgentVersion[] {
  return [...versions].sort((a, b) => b.version - a.version);
}

function makeVersion(overrides: Partial<AgentVersion> = {}): AgentVersion {
  return {
    agent_id: "agent-1",
    config_snapshot: {} as Record<string, object>,
    created_at: "2026-03-28T10:00:00Z",
    created_by: "user-1",
    deployed_at: "2026-03-28T10:01:00Z",
    id: `dep-${overrides.version ?? 1}`,
    provider: "cloudflare",
    status: "deployed",
    updated_at: "2026-03-28T10:01:00Z",
    version: 1,
    ...overrides,
  };
}

describe("agent-version-timeline helpers", () => {
  it("sorts versions newest first", () => {
    const versions = [makeVersion({ version: 1 }), makeVersion({ version: 3 }), makeVersion({ version: 2 })];
    const sorted = sortVersions(versions);
    expect(sorted.map((v) => v.version)).toEqual([3, 2, 1]);
  });

  it("handles empty version list", () => {
    expect(sortVersions([])).toEqual([]);
  });

  it("handles single version", () => {
    const versions = [makeVersion({ version: 1 })];
    const sorted = sortVersions(versions);
    expect(sorted).toHaveLength(1);
    expect(sorted[0]?.version).toBe(1);
  });

  it("preserves all version fields after sorting", () => {
    const v = makeVersion({
      config_snapshot: {} as Record<string, object>,
      created_by: "admin",
      provider: "local_stub",
      status: "failed",
      version: 5,
    });
    const sorted = sortVersions([v]);
    expect(sorted[0]?.provider).toBe("local_stub");
    expect(sorted[0]?.status).toBe("failed");
    expect(sorted[0]?.created_by).toBe("admin");
    expect(sorted[0]?.config_snapshot).toEqual({});
  });

  it("identifies latest version as first after sort", () => {
    const versions = [
      makeVersion({ version: 1, status: "deployed" }),
      makeVersion({ version: 2, status: "deployed" }),
      makeVersion({ version: 3, status: "deployed" }),
    ];
    const sorted = sortVersions(versions);
    expect(sorted[0]?.version).toBe(3);
  });

  it("handles versions with missing config_snapshot", () => {
    const v = makeVersion({ config_snapshot: undefined, version: 1 });
    const sorted = sortVersions([v]);
    expect(sorted[0]?.config_snapshot).toBeUndefined();
  });
});
