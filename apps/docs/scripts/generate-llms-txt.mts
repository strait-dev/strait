/**
 * Generates llms.txt (navigation index) and llms-full.txt (full content)
 * for AI agent discoverability.
 */

import fs from "node:fs";
import path from "node:path";

const CONTENT_DIR = path.resolve(import.meta.dirname, "../content/docs");
const PUBLIC_DIR = path.resolve(import.meta.dirname, "../public");
const BASE_URL = "https://docs.strait.dev/docs";

interface PageEntry {
  title: string;
  description: string;
  section: string;
  slug: string;
  content: string;
}

const SECTION_ORDER = [
  "getting-started",
  "concepts",
  "sdks",
  "integrations",
  "ai",
  "api-reference",
  "cli",
  "guides",
  "operations",
  "development",
];

const FRONTMATTER_RE = /^---\n([\s\S]*?)\n---\n?([\s\S]*)/;
const TITLE_RE = /title:\s*"?([^"\n]+)"?/;
const DESC_RE = /description:\s*"?([^"\n]+)"?/;
const MDX_EXT_RE = /\.mdx$/;

function extractFrontmatter(content: string): {
  title: string;
  description: string;
  body: string;
} {
  const match = content.match(FRONTMATTER_RE);
  if (!match) {
    return { title: "", description: "", body: content };
  }

  const fm = match[1];
  const body = match[2];

  const titleMatch = fm.match(TITLE_RE);
  const descMatch = fm.match(DESC_RE);

  return {
    title: titleMatch ? titleMatch[1].trim() : "",
    description: descMatch ? descMatch[1].trim() : "",
    body,
  };
}

function stripMdxComponents(content: string): string {
  // Remove JSX component tags but keep their text content
  return content
    .replace(/<[A-Z][a-zA-Z]*[^>]*>/g, "")
    .replace(/<\/[A-Z][a-zA-Z]*>/g, "")
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, "") // Remove JSX comments
    .replace(/\n{3,}/g, "\n\n") // Collapse multiple blank lines
    .trim();
}

function collectPages(): PageEntry[] {
  const pages: PageEntry[] = [];

  for (const section of SECTION_ORDER) {
    const sectionDir = path.join(CONTENT_DIR, section);
    if (!fs.existsSync(sectionDir)) {
      continue;
    }

    const files = fs.readdirSync(sectionDir).filter((f) => f.endsWith(".mdx"));
    for (const file of files) {
      const filePath = path.join(sectionDir, file);
      const content = fs.readFileSync(filePath, "utf-8");
      const { title, description, body } = extractFrontmatter(content);
      const slug = file.replace(MDX_EXT_RE, "");
      const urlSlug = slug === "index" ? section : `${section}/${slug}`;

      pages.push({
        title,
        description,
        section,
        slug: urlSlug,
        content: stripMdxComponents(body),
      });
    }
  }

  return pages;
}

function generateLlmsTxt(pages: PageEntry[]): string {
  const lines: string[] = [
    "# Strait Documentation",
    "",
    "> Strait is a production-grade Go job orchestration platform for engineering teams and AI agents.",
    "",
  ];

  let currentSection = "";
  for (const page of pages) {
    if (page.section !== currentSection) {
      currentSection = page.section;
      lines.push(
        `## ${currentSection.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}`
      );
      lines.push("");
    }
    lines.push(
      `- [${page.title}](${BASE_URL}/${page.slug}): ${page.description}`
    );
  }

  return lines.join("\n");
}

function generateLlmsFullTxt(pages: PageEntry[]): string {
  const sections: string[] = [
    "# Strait Documentation - Full Content",
    "",
    "> Strait is a production-grade Go job orchestration platform for engineering teams and AI agents.",
    "",
  ];

  let currentSection = "";
  for (const page of pages) {
    if (page.section !== currentSection) {
      currentSection = page.section;
      sections.push(
        `${"=".repeat(60)}`,
        `## ${currentSection.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}`,
        `${"=".repeat(60)}`,
        ""
      );
    }

    sections.push(`### ${page.title}`);
    if (page.description) {
      sections.push(`> ${page.description}`);
    }
    sections.push(`URL: ${BASE_URL}/${page.slug}`);
    sections.push("");
    sections.push(page.content);
    sections.push("");
    sections.push("---");
    sections.push("");
  }

  return sections.join("\n");
}

function main() {
  const pages = collectPages();

  fs.mkdirSync(PUBLIC_DIR, { recursive: true });

  const llmsTxt = generateLlmsTxt(pages);
  fs.writeFileSync(path.join(PUBLIC_DIR, "llms.txt"), llmsTxt);
  console.log(`Generated llms.txt (${pages.length} pages)`);

  const llmsFullTxt = generateLlmsFullTxt(pages);
  fs.writeFileSync(path.join(PUBLIC_DIR, "llms-full.txt"), llmsFullTxt);
  console.log(`Generated llms-full.txt (${llmsFullTxt.length} bytes)`);
}

main();
