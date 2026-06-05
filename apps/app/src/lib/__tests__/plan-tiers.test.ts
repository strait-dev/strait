import { ROADMAP_FEATURES } from "@strait/billing";
import { describe, expect, it } from "vitest";
import {
  canUseFeature,
  getFeatureMinimumPlanLabel,
  isDowngrade,
  isRoadmapFeature,
  type PlanFeature,
  ROADMAP_FEATURE_LABELS,
  tierAtLeast,
} from "../plan-tiers";

describe("isDowngrade", () => {
  it("returns true when going from pro to free", () => {
    expect(isDowngrade("pro", "free")).toBe(true);
  });

  it("returns true when going from pro to starter", () => {
    expect(isDowngrade("pro", "starter")).toBe(true);
  });

  it("returns true when going from enterprise to pro", () => {
    expect(isDowngrade("enterprise", "pro")).toBe(true);
  });

  it("returns true when going from enterprise to free", () => {
    expect(isDowngrade("enterprise", "free")).toBe(true);
  });

  it("returns true when going from starter to free", () => {
    expect(isDowngrade("starter", "free")).toBe(true);
  });

  it("returns true when going from scale to pro", () => {
    expect(isDowngrade("scale", "pro")).toBe(true);
  });

  it("returns true when going from scale to starter", () => {
    expect(isDowngrade("scale", "starter")).toBe(true);
  });

  it("returns true when going from scale to free", () => {
    expect(isDowngrade("scale", "free")).toBe(true);
  });

  it("returns true when going from enterprise to scale", () => {
    expect(isDowngrade("enterprise", "scale")).toBe(true);
  });

  it("returns false when going from free to starter (upgrade)", () => {
    expect(isDowngrade("free", "starter")).toBe(false);
  });

  it("returns false when going from starter to pro (upgrade)", () => {
    expect(isDowngrade("starter", "pro")).toBe(false);
  });

  it("returns false when going from pro to scale (upgrade)", () => {
    expect(isDowngrade("pro", "scale")).toBe(false);
  });

  it("returns false when going from scale to enterprise (upgrade)", () => {
    expect(isDowngrade("scale", "enterprise")).toBe(false);
  });

  it("returns false when going from free to enterprise (upgrade)", () => {
    expect(isDowngrade("free", "enterprise")).toBe(false);
  });

  it("returns false when going from free to scale (upgrade)", () => {
    expect(isDowngrade("free", "scale")).toBe(false);
  });

  it("returns false for same plan", () => {
    expect(isDowngrade("pro", "pro")).toBe(false);
  });

  it("returns false for same plan (free)", () => {
    expect(isDowngrade("free", "free")).toBe(false);
  });

  it("returns false for same plan (scale)", () => {
    expect(isDowngrade("scale", "scale")).toBe(false);
  });

  it("returns false when currentTier is undefined", () => {
    expect(isDowngrade(undefined, "pro")).toBe(false);
  });

  it("returns false when targetTier is undefined", () => {
    expect(isDowngrade("pro", undefined)).toBe(false);
  });

  it("returns false when both are undefined", () => {
    expect(isDowngrade(undefined, undefined)).toBe(false);
  });

  it("treats unknown tiers as rank 0 (same as free)", () => {
    expect(isDowngrade("unknown", "free")).toBe(false);
    expect(isDowngrade("pro", "unknown")).toBe(true);
  });
});

describe("tierAtLeast", () => {
  it("returns true when tier equals minimum", () => {
    expect(tierAtLeast("pro", "pro")).toBe(true);
  });

  it("returns true when tier is above minimum", () => {
    expect(tierAtLeast("scale", "pro")).toBe(true);
    expect(tierAtLeast("enterprise", "starter")).toBe(true);
  });

  it("returns false when tier is below minimum", () => {
    expect(tierAtLeast("free", "pro")).toBe(false);
    expect(tierAtLeast("starter", "scale")).toBe(false);
  });

  it("returns false for undefined tier", () => {
    expect(tierAtLeast(undefined, "free")).toBe(false);
  });

  it("returns true for free with free minimum", () => {
    expect(tierAtLeast("free", "free")).toBe(true);
  });
});

describe("canUseFeature", () => {
  const freeFeatures: PlanFeature[] = ["http_mode"];

  const starterFeatures: PlanFeature[] = ["log_streaming"];

  const proFeatures: PlanFeature[] = [
    "approval_gates",
    "sub_workflows",
    "job_chaining",
    "compensating_txns",
  ];

  const scaleFeatures: PlanFeature[] = ["canary_deployments", "audit_logs"];

  const businessFeatures: PlanFeature[] = ["sla"];

  const roadmapFeatures: PlanFeature[] = [
    "sso",
    "dedicated_worker_pool",
    "static_ips",
    "vpc_peering",
    "scim",
    "data_residency",
    "ip_allowlisting",
    "single_tenant",
    "byo_cloud",
    "compliance_archive",
  ];

  it("allows free launch features on all plans", () => {
    for (const feature of freeFeatures) {
      expect(canUseFeature("free", feature)).toBe(true);
      expect(canUseFeature("starter", feature)).toBe(true);
      expect(canUseFeature("pro", feature)).toBe(true);
      expect(canUseFeature("enterprise", feature)).toBe(true);
    }
  });

  it("allows starter features on starter and above", () => {
    for (const feature of starterFeatures) {
      expect(canUseFeature("free", feature)).toBe(false);
      expect(canUseFeature("starter", feature)).toBe(true);
      expect(canUseFeature("pro", feature)).toBe(true);
      expect(canUseFeature("enterprise", feature)).toBe(true);
    }
  });

  it("blocks pro features on free and starter", () => {
    for (const feature of proFeatures) {
      expect(canUseFeature("free", feature)).toBe(false);
      expect(canUseFeature("starter", feature)).toBe(false);
    }
  });

  it("allows pro features on pro and above", () => {
    for (const feature of proFeatures) {
      expect(canUseFeature("pro", feature)).toBe(true);
      expect(canUseFeature("scale", feature)).toBe(true);
      expect(canUseFeature("enterprise", feature)).toBe(true);
    }
  });

  it("blocks scale features below scale", () => {
    for (const feature of scaleFeatures) {
      expect(canUseFeature("free", feature)).toBe(false);
      expect(canUseFeature("starter", feature)).toBe(false);
      expect(canUseFeature("pro", feature)).toBe(false);
    }
  });

  it("allows scale features on scale and enterprise", () => {
    for (const feature of scaleFeatures) {
      expect(canUseFeature("scale", feature)).toBe(true);
      expect(canUseFeature("enterprise", feature)).toBe(true);
    }
  });

  it("blocks business features below business", () => {
    for (const feature of businessFeatures) {
      expect(canUseFeature("free", feature)).toBe(false);
      expect(canUseFeature("starter", feature)).toBe(false);
      expect(canUseFeature("pro", feature)).toBe(false);
      expect(canUseFeature("scale", feature)).toBe(false);
    }
  });

  it("allows business features on business and enterprise", () => {
    for (const feature of businessFeatures) {
      expect(canUseFeature("business", feature)).toBe(true);
      expect(canUseFeature("enterprise", feature)).toBe(true);
    }
  });

  it("blocks roadmap-only features below enterprise", () => {
    for (const feature of roadmapFeatures) {
      expect(canUseFeature("free", feature)).toBe(false);
      expect(canUseFeature("starter", feature)).toBe(false);
      expect(canUseFeature("pro", feature)).toBe(false);
      expect(canUseFeature("scale", feature)).toBe(false);
    }
  });

  it("blocks roadmap-only features on enterprise", () => {
    for (const feature of roadmapFeatures) {
      expect(canUseFeature("enterprise", feature)).toBe(false);
      expect(isRoadmapFeature(feature)).toBe(true);
    }
  });

  it("keeps roadmap feature labels in sync with the shared catalog", () => {
    const appLabels = Object.values(ROADMAP_FEATURE_LABELS).sort();
    expect(appLabels).toEqual([...ROADMAP_FEATURES].sort());

    for (const [feature, label] of Object.entries(ROADMAP_FEATURE_LABELS)) {
      expect(ROADMAP_FEATURES).toContain(label);
      expect(canUseFeature("enterprise", feature as PlanFeature)).toBe(false);
    }
  });

  it("returns false for undefined tier", () => {
    expect(canUseFeature(undefined, "http_mode")).toBe(false);
  });
});

describe("getFeatureMinimumPlanLabel", () => {
  it("returns the minimum launch tier label", () => {
    expect(getFeatureMinimumPlanLabel("http_mode")).toBe("Free");
    expect(getFeatureMinimumPlanLabel("log_streaming")).toBe("Starter");
    expect(getFeatureMinimumPlanLabel("audit_logs")).toBe("Scale");
  });

  it("labels roadmap-only features explicitly", () => {
    expect(getFeatureMinimumPlanLabel("sso")).toBe("Roadmap");
  });
});
