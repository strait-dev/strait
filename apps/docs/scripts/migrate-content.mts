/**
 * Migration script: converts Mintlify MDX docs to Fumadocs format.
 *
 * Transforms:
 *   <Info>           -> <Callout type="info">
 *   <Warning>        -> <Callout type="warn">
 *   <Tip>            -> <Callout type="info">
 *   <Note>           -> <Callout type="info">
 *   <Success>        -> <Callout type="info">
 *   <CardGroup ...>  -> <Cards>
 *   <Card title="..." icon="..." href="..."> -> <Card title="..." href="...">
 *   <Steps>/<Step>   -> <Steps>/<Step>  (kept as-is, fumadocs supports them)
 *   <CodeGroup>      -> <Tabs> with <Tab> per code block
 *   <Tabs>/<Tab>     -> <Tabs items={[...]}> / <Tab value="...">
 *   <Troubleshoot>   -> <Accordions>/<Accordion>
 *   <Details>        -> <Accordion>
 *
 * Also rewrites internal links from /docs/path -> relative section paths.
 */

import fs from "node:fs";
import path from "node:path";

const DOCS_SRC = path.resolve(import.meta.dirname, "../../../docs");
const DOCS_DEST = path.resolve(import.meta.dirname, "../content/docs");

// Section mapping: source path -> destination section folder
const SECTION_MAP: Record<string, string> = {
  // Root files -> getting-started
  introduction: "getting-started/index",
  quickstart: "getting-started/quickstart",
  architecture: "getting-started/architecture",
  changelog: "getting-started/changelog",

  // Concepts
  "concepts/jobs": "concepts/jobs",
  "concepts/runs": "concepts/runs",
  "concepts/workflows": "concepts/workflows",
  "concepts/dag-runtime": "concepts/dag-runtime",
  "concepts/scheduling": "concepts/scheduling",
  "concepts/retry-strategies": "concepts/retry-strategies",
  "concepts/resilience": "concepts/resilience",
  "concepts/event-triggers": "concepts/event-triggers",
  "concepts/webhooks": "concepts/webhooks",
  "concepts/environments": "concepts/environments",
  "concepts/versioning": "concepts/versioning",
  "concepts/cost-budgets": "concepts/cost-budgets",
  "concepts/cdc": "concepts/cdc",
  "concepts/adaptive-concurrency": "concepts/adaptive-concurrency",
  "concepts/audit-logging": "concepts/audit-logging",
  "concepts/webhook-subscriptions": "concepts/webhook-subscriptions",
  "concepts/managed-execution": "concepts/managed-execution",
  "concepts/clickhouse-analytics": "concepts/clickhouse-analytics",
  "concepts/log-drains": "concepts/log-drains",
  "concepts/event-sources": "concepts/event-sources",
  "concepts/batch-operations": "concepts/batch-operations",

  // SDKs
  "sdks/overview": "sdks/index",
  "sdks/configuration": "sdks/configuration",
  "sdks/typescript": "sdks/typescript",
  "sdks/python": "sdks/python",
  "sdks/go": "sdks/go",
  "sdks/ruby": "sdks/ruby",
  "sdks/rust": "sdks/rust",

  // API Reference
  "api-reference/overview": "api-reference/index",

  // CLI
  "cli/overview": "cli/index",
  "cli/init": "cli/init",
  "cli/jobs": "cli/jobs",
  "cli/runs": "cli/runs",
  "cli/workflows": "cli/workflows",
  "cli/workflow-runs": "cli/workflow-runs",
  "cli/trigger": "cli/trigger",
  "cli/events": "cli/events",
  "cli/api-keys": "cli/api-keys",
  "cli/secrets": "cli/secrets",
  "cli/stats": "cli/stats",
  "cli/backup": "cli/backup",
  "cli/server": "cli/server",
  "cli/dev": "cli/dev",
  "cli/utilities": "cli/utilities",

  // Guides (including configuration merged in)
  "guides/authentication": "guides/authentication",
  "guides/oidc": "guides/oidc",
  "guides/rbac": "guides/rbac",
  "guides/audit-events": "guides/audit-events",
  "guides/api-key-rotation": "guides/api-key-rotation",
  "guides/security": "guides/security",
  "guides/deployment": "guides/deployment",
  "guides/performance-tuning": "guides/performance-tuning",
  "guides/job-dependencies": "guides/job-dependencies",
  "guides/debug-bundles": "guides/debug-bundles",
  "guides/job-groups": "guides/job-groups",
  "guides/idempotency": "guides/idempotency",
  "guides/sdk-integration": "guides/sdk-integration",
  "guides/workflow-approvals": "guides/workflow-approvals",
  "guides/dag-operations-playbook": "guides/dag-operations-playbook",
  "guides/event-triggers": "guides/event-triggers",
  "configuration/environment-variables": "guides/environment-variables",
  "configuration/database": "guides/database",

  // Operations
  "operations/monitoring-and-alerts": "operations/monitoring-and-alerts",
  "operations/oidc-rollout": "operations/oidc-rollout",
  "operations/rbac-policy-rollout": "operations/rbac-policy-rollout",
  "operations/api-key-rotation-rollout": "operations/api-key-rotation-rollout",
  "operations/incident-response-authz": "operations/incident-response-authz",

  // Development
  "development/contributing": "development/contributing",
  "development/testing": "development/testing",
  "development/database-schema": "development/database-schema",
  "development/technology-choices": "development/technology-choices",
  migrations: "development/migrations",
};

// Build a reverse link map: old URL path -> new URL path
function buildLinkMap(): Map<string, string> {
  const map = new Map<string, string>();
  for (const [src, dest] of Object.entries(SECTION_MAP)) {
    // Old Mintlify link format: /concepts/jobs or concepts/jobs
    // New Fumadocs link: /docs/concepts/jobs (section folder is stripped from URL since root)
    const oldPath = src;
    // Handle index files: getting-started/index -> getting-started
    const urlPath = dest.endsWith("/index") ? dest.replace("/index", "") : dest;
    map.set(oldPath, `/docs/${urlPath}`);
    map.set(`/${oldPath}`, `/docs/${urlPath}`);
    map.set(`/docs/${oldPath}`, `/docs/${urlPath}`);
  }
  return map;
}

function transformContent(
  content: string,
  linkMap: Map<string, string>
): string {
  let result = content;

  // Transform callout components
  result = result.replace(/<Info>/g, '<Callout type="info">');
  result = result.replace(/<\/Info>/g, "</Callout>");
  result = result.replace(/<Warning>/g, '<Callout type="warn">');
  result = result.replace(/<\/Warning>/g, "</Callout>");
  result = result.replace(/<Tip>/g, '<Callout type="info">');
  result = result.replace(/<\/Tip>/g, "</Callout>");
  result = result.replace(/<Note>/g, '<Callout type="info">');
  result = result.replace(/<\/Note>/g, "</Callout>");
  result = result.replace(/<Success>/g, '<Callout type="info">');
  result = result.replace(/<\/Success>/g, "</Callout>");
  result = result.replace(/<Example>/g, '<Callout type="info">');
  result = result.replace(/<\/Example>/g, "</Callout>");

  // Transform CardGroup -> Cards (strip cols attribute)
  result = result.replace(/<CardGroup[^>]*>/g, "<Cards>");
  result = result.replace(/<\/CardGroup>/g, "</Cards>");

  // Transform Card: strip icon attribute, keep title and href
  result = result.replace(
    /<Card\s+title="([^"]*)"(?:\s+icon="[^"]*")?(?:\s+href="([^"]*)")?>/g,
    (_match, title: string, href: string | undefined) => {
      if (href) {
        return `<Card title="${title}" href="${href}">`;
      }
      return `<Card title="${title}">`;
    }
  );

  // Transform Troubleshoot -> Callout (troubleshooting tips are better as callouts)
  result = result.replace(/<Troubleshoot>/g, '<Callout type="warn">');
  result = result.replace(/<\/Troubleshoot>/g, "</Callout>");

  // Transform Details -> just remove the wrapper (content stays inline)
  result = result.replace(/<Details>/g, "");
  result = result.replace(/<\/Details>/g, "");

  // Transform CodeGroup -> extract titles from code block language lines and wrap in Tabs
  result = transformCodeGroups(result);

  // Transform Tabs/Tab (non-CodeGroup)
  result = transformTabs(result);

  // Rewrite internal links
  result = rewriteLinks(result, linkMap);

  return result;
}

const CODE_BLOCK_REGEX = /```(\w+)\s+(.+)\n([\s\S]*?)```/g;

function transformCodeGroups(content: string): string {
  // Match <CodeGroup> ... </CodeGroup> blocks
  return content.replace(
    /<CodeGroup>([\s\S]*?)<\/CodeGroup>/g,
    (_match, inner: string) => {
      // Extract code blocks: ```lang Title\n...\n```
      const tabs: { lang: string; title: string; code: string }[] = [];
      let m = CODE_BLOCK_REGEX.exec(inner);

      while (m !== null) {
        tabs.push({ lang: m[1], title: m[2], code: m[3] });
        m = CODE_BLOCK_REGEX.exec(inner);
      }
      CODE_BLOCK_REGEX.lastIndex = 0;

      if (tabs.length === 0) {
        return inner;
      }

      const items = tabs.map((t) => t.title);
      let result = `<Tabs items={${JSON.stringify(items)}}>\n`;
      for (const tab of tabs) {
        result += `  <Tab value="${tab.title}">\n`;
        result += `    \`\`\`${tab.lang}\n${tab.code}\`\`\`\n`;
        result += "  </Tab>\n";
      }
      result += "</Tabs>";
      return result;
    }
  );
}

const TAB_REGEX = /<Tab\s+title="([^"]*)">([\s\S]*?)<\/Tab>/g;

function transformTabs(content: string): string {
  // Match <Tabs> ... </Tabs> blocks (not already transformed from CodeGroup)
  return content.replace(
    /<Tabs>([\s\S]*?)<\/Tabs>/g,
    (original, inner: string) => {
      const tabs: { title: string; content: string }[] = [];
      let m = TAB_REGEX.exec(inner);

      while (m !== null) {
        tabs.push({ title: m[1], content: m[2] });
        m = TAB_REGEX.exec(inner);
      }
      TAB_REGEX.lastIndex = 0;

      if (tabs.length === 0) {
        return original;
      }

      const items = tabs.map((t) => t.title);
      let result = `<Tabs items={${JSON.stringify(items)}}>\n`;
      for (const tab of tabs) {
        result += `  <Tab value="${tab.title}">${tab.content}</Tab>\n`;
      }
      result += "</Tabs>";
      return result;
    }
  );
}

const DOCS_PREFIX_REGEX = /^\/docs\//;
const FRONTMATTER_REGEX = /^---\n([\s\S]*?)\n---/;

function rewriteLinks(content: string, linkMap: Map<string, string>): string {
  // Rewrite Markdown links: [text](/path) and [text](path)
  return content.replace(
    /\[([^\]]*)\]\(([^)]*)\)/g,
    (match, text: string, href: string) => {
      // Skip external links and anchor-only links
      if (href.startsWith("http") || href.startsWith("#")) {
        return match;
      }
      // Try to find in link map
      const newHref = linkMap.get(href);
      if (newHref) {
        return `[${text}](${newHref})`;
      }
      // Try stripping /docs/ prefix
      const withoutDocs = href.replace(DOCS_PREFIX_REGEX, "");
      const mapped = linkMap.get(withoutDocs);
      if (mapped) {
        return `[${text}](${mapped})`;
      }
      return match;
    }
  );
}

function transformFrontmatter(content: string): string {
  // Remove Mintlify-specific frontmatter keys (openapi, api) if present
  return content.replace(FRONTMATTER_REGEX, (_match, fm: string) => {
    const lines = fm.split("\n").filter((line: string) => {
      const key = line.split(":")[0].trim();
      return !["openapi", "api", "mode"].includes(key);
    });
    return `---\n${lines.join("\n")}\n---`;
  });
}

function main() {
  const linkMap = buildLinkMap();
  let migrated = 0;
  let skipped = 0;
  const errors: string[] = [];

  for (const [srcKey, destPath] of Object.entries(SECTION_MAP)) {
    const srcFile = path.join(DOCS_SRC, `${srcKey}.mdx`);
    const destFile = path.join(DOCS_DEST, `${destPath}.mdx`);

    if (!fs.existsSync(srcFile)) {
      console.log(`SKIP: ${srcKey}.mdx (not found)`);
      skipped++;
      continue;
    }

    let content = fs.readFileSync(srcFile, "utf-8");

    // Transform frontmatter
    content = transformFrontmatter(content);

    // Transform content
    content = transformContent(content, linkMap);

    // Ensure destination directory exists
    fs.mkdirSync(path.dirname(destFile), { recursive: true });

    // Write transformed file
    fs.writeFileSync(destFile, content, "utf-8");
    migrated++;
    console.log(`OK: ${srcKey} -> ${destPath}`);
  }

  console.log(`\nMigration complete: ${migrated} migrated, ${skipped} skipped`);
  if (errors.length > 0) {
    console.log(`Errors:\n${errors.join("\n")}`);
  }

  // Report any untransformed Mintlify components
  checkForRemainingComponents();
}

function checkForRemainingComponents() {
  const mintlifyTags = [
    "Info",
    "Warning",
    "Tip",
    "Note",
    "Success",
    "CardGroup",
    "CodeGroup",
    "Troubleshoot",
    "Details",
    "Example",
  ];

  console.log("\nChecking for untransformed Mintlify components...");
  let found = false;

  function walkDir(dir: string) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const fullPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        walkDir(fullPath);
      } else if (entry.name.endsWith(".mdx")) {
        const content = fs.readFileSync(fullPath, "utf-8");
        for (const tag of mintlifyTags) {
          const regex = new RegExp(`<${tag}[\\s>]`, "g");
          const matches = content.match(regex);
          if (matches) {
            console.log(
              `  WARN: ${path.relative(DOCS_DEST, fullPath)} still has <${tag}> (${matches.length} occurrences)`
            );
            found = true;
          }
        }
      }
    }
  }

  walkDir(DOCS_DEST);

  if (!found) {
    console.log("  All Mintlify components transformed successfully.");
  }
}

main();
