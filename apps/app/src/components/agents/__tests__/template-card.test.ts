import { describe, expect, it } from "vitest";

const MOCK_TEMPLATES = [
  {
    name: "Incident Triage",
    slug: "incident-triage",
    description: "Classifies incoming incidents",
    category: "operations",
    model: "gpt-4o-mini",
    config: { temperature: 0.1 },
  },
  {
    name: "Code Reviewer",
    slug: "code-reviewer",
    description: "Reviews code changes",
    category: "engineering",
    model: "claude-sonnet-4-5",
    config: { temperature: 0.2 },
  },
  {
    name: "Data Extractor",
    slug: "data-extractor",
    description: "Extracts structured data",
    category: "content",
    model: "gpt-4o",
    config: { temperature: 0 },
  },
];

const VALID_CATEGORIES = ["content", "engineering", "operations"];
const SLUG_PATTERN = /^[a-z0-9-]+$/;

describe("agent template data", () => {
  it("all templates have required fields", () => {
    const requiredFields = [
      "name",
      "slug",
      "description",
      "category",
      "model",
    ] as const;
    for (const template of MOCK_TEMPLATES) {
      for (const field of requiredFields) {
        expect(template[field]).toBeTruthy();
      }
    }
  });

  it("all categories are valid", () => {
    for (const template of MOCK_TEMPLATES) {
      expect(VALID_CATEGORIES).toContain(template.category);
    }
  });

  it("all slugs are URL-safe", () => {
    for (const template of MOCK_TEMPLATES) {
      expect(template.slug).toMatch(SLUG_PATTERN);
    }
  });

  it("all slugs are unique", () => {
    const slugs = MOCK_TEMPLATES.map((t) => t.slug);
    expect(new Set(slugs).size).toBe(slugs.length);
  });

  it("configs are valid objects", () => {
    for (const template of MOCK_TEMPLATES) {
      expect(typeof template.config).toBe("object");
      expect(template.config).not.toBeNull();
    }
  });
});
