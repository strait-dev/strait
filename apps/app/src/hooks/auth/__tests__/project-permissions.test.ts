import { describe, expect, it } from "vitest";
import {
  flagsFromPermissions,
  hasScope,
  rolePermissions,
} from "@/hooks/auth/project-permissions";

describe("hasScope", () => {
  it("matches an explicitly granted scope", () => {
    expect(hasScope(["jobs:write"], "jobs:write")).toBe(true);
  });

  it("grants any scope via the wildcard", () => {
    expect(hasScope(["*"], "jobs:write")).toBe(true);
    expect(hasScope(["*"], "anything:at-all")).toBe(true);
  });

  it("denies scopes that are not present", () => {
    expect(hasScope(["runs:write"], "jobs:write")).toBe(false);
    expect(hasScope([], "jobs:write")).toBe(false);
  });
});

describe("flagsFromPermissions", () => {
  it("denies everything for an empty scope list", () => {
    const flags = flagsFromPermissions([]);
    expect(flags).toEqual({
      permissions: [],
      canWriteJobs: false,
      canTriggerJobs: false,
      canWriteRuns: false,
      canWriteWorkflows: false,
      canTriggerWorkflows: false,
      canWriteWebhooks: false,
      canManageApiKeys: false,
      canWriteProjects: false,
      canManageProjects: false,
    });
  });

  it("grants everything for the wildcard scope", () => {
    const flags = flagsFromPermissions(["*"]);
    const { permissions, ...booleans } = flags;
    expect(permissions).toEqual(["*"]);
    expect(Object.values(booleans).every(Boolean)).toBe(true);
  });

  it("treats a write scope as implying the matching trigger scope", () => {
    const jobs = flagsFromPermissions(["jobs:write"]);
    expect(jobs.canWriteJobs).toBe(true);
    expect(jobs.canTriggerJobs).toBe(true);

    const workflows = flagsFromPermissions(["workflows:write"]);
    expect(workflows.canWriteWorkflows).toBe(true);
    expect(workflows.canTriggerWorkflows).toBe(true);
  });

  it("does not treat a trigger scope as implying write", () => {
    const jobs = flagsFromPermissions(["jobs:trigger"]);
    expect(jobs.canTriggerJobs).toBe(true);
    expect(jobs.canWriteJobs).toBe(false);

    const workflows = flagsFromPermissions(["workflows:trigger"]);
    expect(workflows.canTriggerWorkflows).toBe(true);
    expect(workflows.canWriteWorkflows).toBe(false);
  });

  it("maps management scopes without leaking into write scopes", () => {
    const apiKeys = flagsFromPermissions(["api-keys:manage"]);
    expect(apiKeys.canManageApiKeys).toBe(true);
    expect(apiKeys.canWriteJobs).toBe(false);

    const projects = flagsFromPermissions(["projects:manage"]);
    expect(projects.canManageProjects).toBe(true);
    expect(projects.canWriteProjects).toBe(false);
  });
});

describe("rolePermissions", () => {
  it("returns a flat role's scopes", () => {
    expect(
      rolePermissions({ permissions: ["jobs:write", "runs:write"] })
    ).toEqual(["jobs:write", "runs:write"]);
  });

  it("treats a flat role with null permissions as empty", () => {
    expect(rolePermissions({ permissions: null })).toEqual([]);
  });

  it("merges a role's own scopes with every inherited role's scopes", () => {
    expect(
      rolePermissions({
        role: { permissions: ["jobs:write"] },
        lineage: [
          { permissions: ["runs:write"] },
          { permissions: ["webhooks:write"] },
        ],
      })
    ).toEqual(["jobs:write", "runs:write", "webhooks:write"]);
  });

  it("handles a lineage response with missing role and null scopes", () => {
    expect(
      rolePermissions({
        lineage: [{ permissions: null }, { permissions: ["x"] }],
      })
    ).toEqual(["x"]);
    expect(rolePermissions({ role: { permissions: null } })).toEqual([]);
  });
});
