import { describe, expect, it } from "vitest";
import {
  enterpriseContactSchema,
  USE_CASES,
} from "@/lib/enterprise-contact-schema";
import { FeedbackFormSchema, SupportFormSchema } from "@/lib/schema";

const validEmail = "user@example.com";

describe("FeedbackFormSchema", () => {
  it("accepts known subjects and bounded messages", () => {
    const result = FeedbackFormSchema.safeParse({
      email: validEmail,
      subject: "feedback",
      message: "This is useful feedback.",
    });

    expect(result.success).toBe(true);
  });

  it("rejects unexpected subjects and oversized messages", () => {
    expect(
      FeedbackFormSchema.safeParse({
        email: validEmail,
        subject: "../admin",
        message: "This is useful feedback.",
      }).success
    ).toBe(false);

    expect(
      FeedbackFormSchema.safeParse({
        email: validEmail,
        subject: "bug",
        message: "a".repeat(4001),
      }).success
    ).toBe(false);
  });
});

describe("SupportFormSchema", () => {
  const validSupportPayload = {
    email: validEmail,
    subject: "technical",
    priority: "medium",
    environment: "production",
    message: "The dashboard is not updating correctly.",
    steps_to_reproduce: "Open the dashboard and wait for a refresh.",
    expected_result: "The chart should show the latest workflow step.",
    actual_result: "The chart remains stuck on the previous step.",
  };

  it("accepts bounded support requests", () => {
    expect(SupportFormSchema.safeParse(validSupportPayload).success).toBe(true);
  });

  it("rejects invalid enum values and oversized details", () => {
    expect(
      SupportFormSchema.safeParse({
        ...validSupportPayload,
        priority: "urgent",
      }).success
    ).toBe(false);

    expect(
      SupportFormSchema.safeParse({
        ...validSupportPayload,
        message: "a".repeat(4001),
      }).success
    ).toBe(false);
  });
});

describe("enterpriseContactSchema", () => {
  const validEnterprisePayload = {
    name: "Jane Smith",
    email: validEmail,
    company: "Acme Inc.",
    teamSize: "51-200",
    useCase: "Roadmap isolation requirements",
    expectedSpend: "$1,500 - $4,000/mo",
    message: "We need help planning a higher-volume Strait deployment.",
  };

  it("accepts known enterprise contact options", () => {
    expect(
      enterpriseContactSchema.safeParse(validEnterprisePayload).success
    ).toBe(true);
  });

  it("rejects arbitrary option values and oversized messages", () => {
    expect(
      enterpriseContactSchema.safeParse({
        ...validEnterprisePayload,
        teamSize: "all of engineering",
      }).success
    ).toBe(false);

    expect(
      enterpriseContactSchema.safeParse({
        ...validEnterprisePayload,
        message: "a".repeat(4001),
      }).success
    ).toBe(false);
  });

  it("keeps unfinished enterprise capabilities labeled as roadmap", () => {
    expect(USE_CASES).toContain("Roadmap security / compliance");
    expect(USE_CASES).toContain("Roadmap isolation requirements");
    expect(USE_CASES).toContain("Roadmap residency requirements");

    expect(USE_CASES).not.toContain("SSO / compliance requirements");
    expect(USE_CASES).not.toContain("Dedicated infrastructure");
    expect(USE_CASES).not.toContain("Data residency");
  });
});
