import { PLANS } from "@strait/billing/products";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EnterpriseOverview } from "../enterprise-overview";

const renderEnterpriseOverview = () =>
  render(
    <EnterpriseOverview
      contractEndDate="2026-12-31"
      enterpriseTier="enterprise"
      overageDiscountPct={15}
      periodSpendMicro={123_456_789}
      slaUptimePct={99.99}
    />
  );

describe("EnterpriseOverview", () => {
  it("renders roadmap features from the generated billing catalog", () => {
    renderEnterpriseOverview();

    for (const feature of PLANS.enterprise.roadmapFeatures) {
      expect(screen.getByText(feature)).toBeTruthy();
    }

    expect(screen.queryByText("SCIM directory sync")).toBeNull();
    expect(screen.queryByText("Static egress IPs")).toBeNull();
    expect(screen.queryByText("Data residency")).toBeNull();
    expect(screen.queryByText("Single-tenant orchestration")).toBeNull();
  });
});
