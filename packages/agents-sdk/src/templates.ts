export type AgentTemplateCategory = "content" | "engineering" | "operations";

export type AgentTemplate = {
  category: AgentTemplateCategory;
  config: Record<string, unknown>;
  description: string;
  model: string;
  name: string;
  slug: string;
};

export const agentTemplates: AgentTemplate[] = [
  {
    name: "Incident Triage",
    slug: "incident-triage",
    description:
      "Classifies incoming incidents by severity and suggests relevant runbooks based on error signatures and service context.",
    model: "gpt-5.4-mini",
    category: "operations",
    config: {
      temperature: 0.1,
      max_attempts: 3,
      timeout_secs: 120,
    },
  },
  {
    name: "Content Classifier",
    slug: "content-classifier",
    description:
      "Categorizes text content using a configurable taxonomy. Supports multi-label classification with confidence scores.",
    model: "gpt-5.4-mini",
    category: "content",
    config: {
      temperature: 0.0,
      max_attempts: 2,
      timeout_secs: 60,
    },
  },
  {
    name: "Code Reviewer",
    slug: "code-reviewer",
    description:
      "Reviews pull requests for security vulnerabilities, performance regressions, and style consistency. Returns structured findings.",
    model: "claude-sonnet-4-6",
    category: "engineering",
    config: {
      temperature: 0.2,
      max_attempts: 2,
      timeout_secs: 300,
      budget: "$1.00",
    },
  },
  {
    name: "Data Extractor",
    slug: "data-extractor",
    description:
      "Extracts structured data from unstructured text, PDFs, and documents. Outputs JSON matching a user-defined schema.",
    model: "gpt-5.4-mini",
    category: "content",
    config: {
      temperature: 0.0,
      max_attempts: 2,
      timeout_secs: 90,
    },
  },
  {
    name: "Support Router",
    slug: "support-router",
    description:
      "Routes incoming support tickets to the appropriate team based on content analysis, urgency detection, and historical patterns.",
    model: "gpt-5.4-mini",
    category: "operations",
    config: {
      temperature: 0.1,
      max_attempts: 2,
      timeout_secs: 30,
    },
  },
];
