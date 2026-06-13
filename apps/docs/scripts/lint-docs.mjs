#!/usr/bin/env node
// Docs consistency linter. Runnable with `bun scripts/lint-docs.mjs` or
// `node scripts/lint-docs.mjs` from apps/docs. Exits non-zero on any finding.
//
// Checks:
//   1. Frontmatter has title and description
//   2. No em-dash or en-dash (house style uses ASCII hyphens)
//   3. No marketing buzzwords
//   4. Every opening code fence has a language tag
//   5. Internal links resolve to a file or a docs.json entry
//   6. Anchor links resolve to a heading in the target page
//   7. No de-normalized example hosts
//   8. No orphan pages (every .mdx is referenced in docs.json)
//   9. Documented HTTP methods resolve against Huma route registration sources
//  10. Documented env vars, run states, and webhook events match Go sources
//  11. First-party Markdown links and README commands resolve

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// ---- regexes (top-level for performance) -----------------------------------
const MDX_EXT_RE = /\.mdx$/;
const TITLE_RE = /^title:\s*\S/;
const DESC_RE = /^description:\s*\S/;
const HEADING_RE = /^#{1,6}\s+(.*)$/;
const BACKTICK_RE = /`/g;
const NON_SLUG_RE = /[^a-z0-9 -]/g;
const MULTI_SPACE_RE = / +/g;
const LEADING_SLASH_RE = /^\//;
const LINK_RE = /(?:\]\(|href=")(\/[a-zA-Z0-9/_#-]+)/g;
const MARKDOWN_LINK_RE = /!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;
const DOC_HTTP_URL_RE =
  /\b(GET|POST|PUT|PATCH|DELETE)\s+(?:https:\/\/api\.strait\.dev|https:\/\/your-strait-instance|http:\/\/localhost:8080)?(\/(?:v1|sdk\/v1)\/[^\s`'")]+)/g;
const CURL_METHOD_RE =
  /\bcurl(?:\s+-[A-Za-z]+[^\n\\]*)*\s+(?:https:\/\/api\.strait\.dev|https:\/\/your-strait-instance|http:\/\/localhost:8080)(\/(?:v1|sdk\/v1)\/[^\s\\'"]+)/g;
const CURL_EXPLICIT_METHOD_RE =
  /\bcurl(?:\s+-[A-Za-z]+[^\n\\]*)*\s+-X\s+(GET|POST|PUT|PATCH|DELETE)\s+(?:https:\/\/api\.strait\.dev|https:\/\/your-strait-instance|http:\/\/localhost:8080)?(\/(?:v1|sdk\/v1)\/[^\s\\'"]+)/g;
const ENV_TAG_RE = /`env:"([A-Z0-9_]+)"`/g;
const GO_STRING_CONST_RE =
  /\b([A-Za-z0-9_]+)\s+(?:[A-Za-z0-9_]+)?\s*=\s*"([^"]+)"/g;
const CODE_TABLE_VALUE_RE = /^`([^`]+)`$/;
const BUN_RUN_RE = /\bbun run(?: --cwd ([^\s]+))?\s+([a-zA-Z0-9:_-]+)/g;
const BUN_DIRECT_RE = /\bbun\s+(dev|build|start|test)\b/g;

// ---- config ----------------------------------------------------------------
const BUZZWORDS = [
  "seamless",
  "leverage",
  "robust",
  "powerful",
  "cutting-edge",
  "best-in-class",
  "effortless",
  "blazing",
  "supercharge",
  "unleash",
  "game-chang",
  "world-class",
  "revolutionary",
  "next-generation",
  "turbocharge",
];
const FORBIDDEN_HOSTS = [
  "https://strait.dev/v1",
  "https://your-strait/",
  "https://your-strait.example",
  "https://strait.example.com",
];

const DOCS = join(dirname(fileURLToPath(import.meta.url)), "..");
const REPO = join(DOCS, "..", "..");
const PRICING_CATALOG = JSON.parse(
  readFileSync(
    join(REPO, "packages", "billing", "catalog", "strait-pricing.json"),
    "utf8"
  )
);
const CONFIG_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "config", "config.go"),
  "utf8"
);
const HUMA_REGISTRY_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "api", "huma_registry.go"),
  "utf8"
);
const HUMA_OPERATIONS_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "api", "huma_operations.go"),
  "utf8"
);
const DOMAIN_TYPES_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "domain", "types.go"),
  "utf8"
);
const WEBHOOK_SUBSCRIPTIONS_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "api", "webhook_subscriptions.go"),
  "utf8"
);
const errors = [];
const err = (file, line, msg) => {
  errors.push({ file, line, msg });
};

// ---- collect mdx files -----------------------------------------------------
function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name.startsWith(".")) {
      continue;
    }
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) {
      walk(p, out);
    } else if (name.endsWith(".mdx")) {
      out.push(p);
    }
  }
  return out;
}
const files = walk(DOCS);
const rel = (p) => relative(DOCS, p);
const repoRel = (p) => relative(REPO, p);

function walkMarkdown(dir, out = []) {
  for (const name of readdirSync(dir)) {
    if (
      name === "node_modules" ||
      name === ".git" ||
      name === ".turbo" ||
      name === ".next" ||
      name === "dist" ||
      name === "build"
    ) {
      continue;
    }
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) {
      if (name.startsWith(".")) {
        continue;
      }
      walkMarkdown(p, out);
    } else if (name.endsWith(".md")) {
      out.push(p);
    }
  }
  return out;
}
const firstPartyMarkdownFiles = walkMarkdown(REPO);

// ---- docs.json: all string scalars + route resolution ----------------------
const docsJson = JSON.parse(readFileSync(join(DOCS, "docs.json"), "utf8"));
const navStrings = new Set();
(function collect(node) {
  if (typeof node === "string") {
    navStrings.add(node);
  } else if (Array.isArray(node)) {
    node.forEach(collect);
  } else if (node && typeof node === "object") {
    Object.values(node).forEach(collect);
  }
})(docsJson);

const routeOf = (p) => {
  let r = rel(p).replace(MDX_EXT_RE, "");
  if (r.endsWith("/index")) {
    r = r.slice(0, -"/index".length);
  }
  return r;
};
const rawRouteOf = (p) => rel(p).replace(MDX_EXT_RE, "");
const routeSet = new Set(files.map(routeOf));

const slugify = (h) =>
  h
    .replace(BACKTICK_RE, "")
    .toLowerCase()
    .replace(NON_SLUG_RE, "")
    .trim()
    .replace(MULTI_SPACE_RE, "-");
const headingSlugs = (p) => {
  const slugs = new Set();
  for (const l of readFileSync(p, "utf8").split("\n")) {
    const m = HEADING_RE.exec(l);
    if (m) {
      slugs.add(slugify(m[1]));
    }
  }
  return slugs;
};

// ---- per-line checks -------------------------------------------------------
function checkFrontmatter(r, text, lines) {
  if (!text.startsWith("---")) {
    err(r, 1, "missing frontmatter block");
    return;
  }
  const end = lines.indexOf("---", 1);
  const fm = lines.slice(1, end === -1 ? 1 : end);
  if (!fm.some((l) => TITLE_RE.test(l))) {
    err(r, 1, "frontmatter missing title");
  }
  if (!fm.some((l) => DESC_RE.test(l))) {
    err(r, 1, "frontmatter missing description");
  }
}

function checkHosts(r, ln, line) {
  for (const h of FORBIDDEN_HOSTS) {
    if (line.includes(h)) {
      err(r, ln, `de-normalized host '${h}'`);
    }
  }
}

function checkLinks(r, ln, line) {
  for (const match of line.matchAll(LINK_RE)) {
    const target = match[1];
    const [path, anchor] = target.split("#");
    if (!path) {
      continue;
    }
    const route = path.replace(LEADING_SLASH_RE, "");
    const ok =
      routeSet.has(route) || navStrings.has(path) || navStrings.has(route);
    if (!ok) {
      err(r, ln, `broken internal link '${target}'`);
      continue;
    }
    if (anchor && routeSet.has(route)) {
      const targetFile = files.find((f) => routeOf(f) === route);
      if (targetFile && !headingSlugs(targetFile).has(anchor)) {
        err(r, ln, `link '${target}' has no matching heading '#${anchor}'`);
      }
    }
  }
}

function checkProse(r, ln, line) {
  if (line.includes("—")) {
    err(r, ln, "em-dash (use ASCII hyphen or rewrite)");
  }
  if (line.includes("–")) {
    err(r, ln, "en-dash (use ASCII hyphen or rewrite)");
  }
  const low = line.toLowerCase();
  for (const w of BUZZWORDS) {
    if (low.includes(w)) {
      err(r, ln, `buzzword '${w}'`);
    }
  }
  checkHosts(r, ln, line);
  checkLinks(r, ln, line);
}

// Returns the next fenced-code state after processing one line.
function checkLine(r, line, ln, inFence) {
  const fence = line.trim();
  if (fence.startsWith("```")) {
    if (inFence) {
      return false;
    }
    if (fence.slice(3).trim() === "") {
      err(r, ln, "code fence has no language tag");
    }
    return true;
  }
  if (inFence) {
    // host hygiene still applies inside code (examples live there)
    checkHosts(r, ln, line);
    return true;
  }
  checkProse(r, ln, line);
  return false;
}

// ---- per-file checks -------------------------------------------------------
for (const p of files) {
  const text = readFileSync(p, "utf8");
  const lines = text.split("\n");
  const r = rel(p);
  checkFrontmatter(r, text, lines);

  let inFence = false;
  for (let i = 0; i < lines.length; i++) {
    inFence = checkLine(r, lines[i], i + 1, inFence);
  }
}

// ---- orphan pages ----------------------------------------------------------
for (const p of files) {
  const route = routeOf(p);
  const raw = rawRouteOf(p);
  const referenced = [route, `/${route}`, raw, `/${raw}`].some((s) =>
    navStrings.has(s)
  );
  if (!referenced) {
    err(rel(p), 1, "orphan page: not referenced in docs.json");
  }
}

// ---- billing catalog drift checks -----------------------------------------
const cents = (value) =>
  value < 0
    ? "Custom"
    : `$${(value / 100).toLocaleString("en-US", { maximumFractionDigits: 0 })}`;
const annual = (value) => (value === 0 ? "-" : cents(value));
const moneyMicros = (value) =>
  value < 0
    ? "Contract"
    : `$${(value / 1_000_000).toLocaleString("en-US", {
        maximumFractionDigits: 0,
      })}`;
const overage = (value) =>
  value < 0 ? "Custom" : `$${(value / 1_000_000).toFixed(2)}`;
const compact = (value) => {
  if (value < 0) {
    return "Unlimited";
  }
  if (value >= 1_000_000) {
    const m = value / 1_000_000;
    return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
  }
  if (value >= 1000) {
    const k = value / 1000;
    return `${Number.isInteger(k) ? k : k.toFixed(1)}K`;
  }
  return String(value);
};
const limit = (value) =>
  value < 0 ? "Unlimited" : value.toLocaleString("en-US");
const retention = (days) =>
  days < 0 ? "Unlimited" : `${days} day${days === 1 ? "" : "s"}`;
const cronInterval = (seconds) => {
  if (seconds === 0) {
    return "sub-second";
  }
  if (seconds < 60) {
    return `${seconds} sec`;
  }
  return `${seconds / 60} min`;
};
const yesNo = (value) => (value ? "Yes" : "No");
const rbac = (level) => {
  if (level === "none") {
    return "None";
  }
  return `${level.charAt(0).toUpperCase()}${level.slice(1)}`;
};
const support = {
  community: "Community",
  email_72h: "Email 72h",
  priority_24h: "Priority 24h",
  priority_slack_8h: "Slack 8h",
  dedicated: "Dedicated",
};
const planLabel = (tier) =>
  tier.charAt(0).toUpperCase() + tier.slice(1).replaceAll("_", " ");
const proseList = (values) =>
  values.length <= 1
    ? values.join("")
    : `${values.slice(0, -1).join(", ")}, and ${values.at(-1)}`;

function parseTableRows(text) {
  const rows = new Map();
  for (const line of text.split("\n")) {
    if (!line.startsWith("|") || line.includes("---")) {
      continue;
    }
    const cells = line
      .split("|")
      .slice(1, -1)
      .map((cell) => cell.trim());
    if (cells.length > 1) {
      rows.set(cells[0], cells.slice(1));
    }
  }
  return rows;
}

function expectPricingRow(file, rows, name, expected) {
  const actual = rows.get(name);
  if (!actual) {
    err(file, 1, `pricing table missing row '${name}'`);
    return;
  }
  if (actual.join(" | ") !== expected.join(" | ")) {
    err(
      file,
      1,
      `pricing row '${name}' = '${actual.join(" | ")}', want '${expected.join(" | ")}'`
    );
  }
}

function expectText(file, text, snippet, label) {
  if (!text.includes(snippet)) {
    err(file, 1, `missing ${label}: '${snippet}'`);
  }
}

// ---- first-party Markdown checks ------------------------------------------
function isExternalLink(target) {
  return (
    target.startsWith("http://") ||
    target.startsWith("https://") ||
    target.startsWith("mailto:") ||
    target.startsWith("#")
  );
}

function stripLinkTarget(target) {
  return target.split("#")[0].split("?")[0];
}

function checkOneMarkdownLink(file, text, target, matchText) {
  if (isExternalLink(target)) {
    return;
  }
  const withoutAnchor = stripLinkTarget(target);
  if (!withoutAnchor) {
    return;
  }
  const resolved = resolve(dirname(file), withoutAnchor);
  if (!(resolved.startsWith(REPO) && existsSync(resolved))) {
    err(
      repoRel(file),
      lineOf(text, matchText),
      `broken Markdown link '${target}'`
    );
    return;
  }
  const anchor = target.includes("#") ? target.split("#").at(-1) : "";
  if (anchor && [".md", ".mdx"].includes(extname(resolved))) {
    const slugs = headingSlugs(resolved);
    if (!slugs.has(anchor)) {
      err(
        repoRel(file),
        lineOf(text, matchText),
        `link '${target}' has no matching heading '#${anchor}'`
      );
    }
  }
}

function checkFirstPartyMarkdownLinks() {
  for (const file of firstPartyMarkdownFiles) {
    if (!existsSync(file)) {
      err(repoRel(file), 1, "configured Markdown file does not exist");
      continue;
    }
    const text = readFileSync(file, "utf8");
    for (const match of text.matchAll(MARKDOWN_LINK_RE)) {
      const target = match[1];
      checkOneMarkdownLink(file, text, target, match[0]);
    }
  }
}

function nearestPackageJSON(file) {
  let dir = dirname(file);
  while (dir.startsWith(REPO)) {
    const candidate = join(dir, "package.json");
    if (existsSync(candidate)) {
      return candidate;
    }
    const next = dirname(dir);
    if (next === dir) {
      break;
    }
    dir = next;
  }
  return join(REPO, "package.json");
}

function packageScripts(packageJSONPath) {
  const pkg = JSON.parse(readFileSync(packageJSONPath, "utf8"));
  return new Set(Object.keys(pkg.scripts ?? {}));
}

function checkBunScript(script, packageJSONPath, file, line) {
  const scripts = packageScripts(packageJSONPath);
  if (!scripts.has(script)) {
    err(
      repoRel(file),
      line,
      `README command references missing script '${script}' in ${repoRel(
        packageJSONPath
      )}`
    );
  }
}

function checkReadmeCommands() {
  for (const file of firstPartyMarkdownFiles.filter(
    (file) => file.endsWith("README.md") || file.endsWith("CONTRIBUTING.md")
  )) {
    if (!existsSync(file)) {
      continue;
    }
    const text = readFileSync(file, "utf8");
    const fallbackPackageJSON = nearestPackageJSON(file);
    for (const match of text.matchAll(BUN_RUN_RE)) {
      const [, cwd, script] = match;
      const packageJSONPath = cwd
        ? join(REPO, cwd, "package.json")
        : fallbackPackageJSON;
      if (!existsSync(packageJSONPath)) {
        err(
          repoRel(file),
          lineOf(text, match[0]),
          `README command references missing package.json at ${repoRel(
            packageJSONPath
          )}`
        );
        continue;
      }
      checkBunScript(script, packageJSONPath, file, lineOf(text, match[0]));
    }
    for (const match of text.matchAll(BUN_DIRECT_RE)) {
      const [, script] = match;
      checkBunScript(script, fallbackPackageJSON, file, lineOf(text, match[0]));
    }
  }
}

function checkAgentDocsSync() {
  const agents = readFileSync(join(REPO, "AGENTS.md"), "utf8");
  const claude = readFileSync(join(REPO, "CLAUDE.md"), "utf8");
  if (agents !== claude) {
    err("AGENTS.md", 1, "AGENTS.md and CLAUDE.md must stay in sync");
  }
}

// ---- source-backed truth checks -------------------------------------------
function lineOf(text, needle) {
  const idx = text.indexOf(needle);
  if (idx === -1) {
    return 1;
  }
  return text.slice(0, idx).split("\n").length;
}

function stripTrailingPunctuation(path) {
  return path.replace(/[.,;:]+$/g, "");
}

function pathPattern(path) {
  const escaped = path
    .replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
    .replace(/\\\{[^/]+\\\}/g, "[^/]+");
  return new RegExp(`^${escaped}$`);
}

const humaRouteRe =
  /Method:\s*http\.Method(Get|Post|Put|Patch|Delete),\s*Path:\s*"([^"]+)"/g;
const openApiRoutes = [...HUMA_REGISTRY_GO.matchAll(humaRouteRe)].map(
  ([, method, path]) => ({
    method: method.toUpperCase(),
    path,
    pattern: pathPattern(path),
  })
);
openApiRoutes.push(
  ...[...HUMA_OPERATIONS_GO.matchAll(humaRouteRe)].map(([, method, path]) => ({
    method: method.toUpperCase(),
    path,
    pattern: pathPattern(path),
  }))
);

function methodExists(method, path) {
  const cleanPath = stripTrailingPunctuation(path.split("?")[0]);
  return openApiRoutes.some(
    (route) => route.method === method && route.pattern.test(cleanPath)
  );
}

function checkDocumentedHttpMethods() {
  for (const p of files) {
    const text = readFileSync(p, "utf8");
    const r = rel(p);
    for (const match of text.matchAll(DOC_HTTP_URL_RE)) {
      const [, method, path] = match;
      if (!methodExists(method, path)) {
        err(r, lineOf(text, match[0]), `unknown API route '${method} ${path}'`);
      }
    }
    for (const match of text.matchAll(CURL_EXPLICIT_METHOD_RE)) {
      const [, method, path] = match;
      if (!methodExists(method, path)) {
        err(r, lineOf(text, match[0]), `unknown API route '${method} ${path}'`);
      }
    }
    for (const match of text.matchAll(CURL_METHOD_RE)) {
      if (match[0].includes(" -X ")) {
        continue;
      }
      const [, path] = match;
      if (!methodExists("GET", path)) {
        err(r, lineOf(text, match[0]), `unknown API route 'GET ${path}'`);
      }
    }
  }
}

function checkEnvCoverage() {
  const envVars = new Set();
  for (const match of CONFIG_GO.matchAll(ENV_TAG_RE)) {
    envVars.add(match[1]);
  }
  const envDocsFile = "configuration/environment-variables.mdx";
  const envDocsText = readFileSync(join(DOCS, envDocsFile), "utf8");
  const envExampleFile = ".env.example";
  const envExampleText = readFileSync(join(REPO, envExampleFile), "utf8");

  for (const envVar of [...envVars].sort()) {
    if (!envDocsText.includes(`\`${envVar}\``)) {
      err(envDocsFile, 1, `missing documented env var '${envVar}'`);
    }
    if (!envExampleText.includes(envVar)) {
      err(envExampleFile, 1, `missing example env var '${envVar}'`);
    }
  }
}

function stringConstMap() {
  const constants = new Map();
  for (const match of DOMAIN_TYPES_GO.matchAll(GO_STRING_CONST_RE)) {
    constants.set(match[1], match[2]);
  }
  return constants;
}

const domainConstants = stringConstMap();

function valuesByConstPrefix(prefix) {
  return new Set(
    [...domainConstants.entries()]
      .filter(([name]) => name.startsWith(prefix))
      .map(([, value]) => value)
  );
}

function tableCodeValues(text) {
  const values = new Set();
  for (const cell of parseTableRows(text).keys()) {
    const m = CODE_TABLE_VALUE_RE.exec(cell);
    if (m) {
      values.add(m[1]);
    }
  }
  return values;
}

function sectionText(text, startHeading, endHeading) {
  const start = text.indexOf(startHeading);
  if (start === -1) {
    return text;
  }
  const end = text.indexOf(endHeading, start + startHeading.length);
  return text.slice(start, end === -1 ? undefined : end);
}

function checkRunStateDocs() {
  const file = "concepts/runs.mdx";
  const text = readFileSync(join(DOCS, file), "utf8");
  const documented = tableCodeValues(
    sectionText(text, "## Run States", "## Trigger a Run")
  );
  const source = valuesByConstPrefix("Status");
  for (const status of documented) {
    if (!source.has(status)) {
      err(
        file,
        lineOf(text, `\`${status}\``),
        `unknown run status '${status}'`
      );
    }
  }
  for (const status of [...source].sort()) {
    if (!documented.has(status)) {
      err(file, 1, `missing run status '${status}'`);
    }
  }
}

function validWebhookEvents() {
  const events = new Set();
  for (const match of WEBHOOK_SUBSCRIPTIONS_GO.matchAll(
    /domain\.(WebhookEvent[A-Za-z0-9_]+):\s+true/g
  )) {
    const value = domainConstants.get(match[1]);
    if (value) {
      events.add(value);
    }
  }
  return events;
}

function checkWebhookEventDocs() {
  const file = "concepts/webhook-subscriptions.mdx";
  const text = readFileSync(join(DOCS, file), "utf8");
  const documented = tableCodeValues(text);
  const source = validWebhookEvents();
  for (const event of documented) {
    if (event.includes(".") && !source.has(event)) {
      err(
        file,
        lineOf(text, `\`${event}\``),
        `unknown webhook event '${event}'`
      );
    }
  }
  for (const event of [...source].sort()) {
    if (!documented.has(event)) {
      err(file, 1, `missing webhook event '${event}'`);
    }
  }
}

function checkBillingCatalogDocs() {
  const pricingFile = "billing/pricing.mdx";
  const faqFile = "billing/faq.mdx";
  const webhookFile = "concepts/webhook-subscriptions.mdx";
  const pricingText = readFileSync(join(DOCS, pricingFile), "utf8");
  const faqText = readFileSync(join(DOCS, faqFile), "utf8");
  const webhookText = readFileSync(join(DOCS, webhookFile), "utf8");
  const rows = parseTableRows(pricingText);
  const plans = PRICING_CATALOG.plans;
  const selfServe = plans.filter((plan) => plan.tier !== "enterprise");
  const enterprise = plans.find((plan) => plan.tier === "enterprise");
  const all = [...selfServe, enterprise];

  expectPricingRow(pricingFile, rows, "Monthly price", [
    ...selfServe.map((plan) => cents(plan.prices.monthlyCents)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Annual price", [
    ...selfServe.map((plan) => annual(plan.prices.annualCents)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Included orchestration runs/mo", [
    ...selfServe.map((plan) => compact(plan.limits.runsPerMonth)),
    "50M+",
  ]);
  expectPricingRow(pricingFile, rows, "Overage per 1K runs", [
    ...selfServe.map((plan) => overage(plan.overage.microusdPer1K)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Default spending cap", [
    "$50 when enabled",
    ...selfServe
      .slice(1)
      .map((plan) => moneyMicros(plan.overage.defaultSpendingCapMicrousd)),
    "Contract",
  ]);
  expectPricingRow(pricingFile, rows, "Concurrent runs", [
    ...selfServe.map((plan) => limit(plan.limits.concurrentRuns)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Max workflow steps", [
    ...selfServe.map((plan) => limit(plan.limits.workflowSteps)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Run history", [
    ...selfServe.map((plan) => retention(plan.limits.retentionDays)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Projects", [
    ...selfServe.map((plan) => limit(plan.limits.projects)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Active environments", [
    ...selfServe.map((plan) => limit(plan.limits.environments)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Cron schedules", [
    ...selfServe.map((plan) => limit(plan.limits.scheduledJobs)),
    "Custom",
  ]);
  expectPricingRow(
    pricingFile,
    rows,
    "Cron minimum interval",
    all.map((plan) => cronInterval(plan.limits.cronMinIntervalSec))
  );
  expectPricingRow(pricingFile, rows, "Members", [
    ...selfServe.map((plan) => limit(plan.limits.members)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Webhook endpoints", [
    ...selfServe.map((plan) => limit(plan.limits.webhookEndpoints)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "Worker connections", [
    ...selfServe.map((plan) => limit(plan.limits.workerConnections)),
    "Custom",
  ]);
  expectPricingRow(pricingFile, rows, "API rate limit", [
    ...selfServe.map((plan) =>
      plan.limits.apiRateLimit < 0
        ? "Unlimited"
        : `${limit(plan.limits.apiRateLimit)}/min`
    ),
    "Custom",
  ]);
  expectPricingRow(
    pricingFile,
    rows,
    "Log streaming",
    all.map((plan) => yesNo(plan.features.logStreaming))
  );
  expectPricingRow(pricingFile, rows, "Log drains", [
    ...selfServe.map((plan) => limit(plan.limits.logDrains)),
    "Custom",
  ]);
  expectPricingRow(
    pricingFile,
    rows,
    "RBAC",
    all.map((plan) => rbac(plan.features.rbacLevel))
  );
  expectPricingRow(
    pricingFile,
    rows,
    "Audit logs",
    all.map((plan) => yesNo(plan.features.auditLogs))
  );
  expectPricingRow(
    pricingFile,
    rows,
    "Canary deployments",
    all.map((plan) => yesNo(plan.features.canaryDeployments))
  );
  expectPricingRow(pricingFile, rows, "SLA target", [
    ...selfServe.slice(0, -1).map(() => "No"),
    "Non-contractual target",
    "Non-contractual target",
  ]);
  expectPricingRow(
    pricingFile,
    rows,
    "Support",
    all.map((plan) => support[plan.supportLevel])
  );

  for (const addon of PRICING_CATALOG.addons) {
    if (addon.status === "active") {
      const display = {
        concurrency_100: "Additional concurrency, +100",
        history_30d: "Extended run history, +30d",
        environments_5: "Additional environments, +5",
      }[addon.type];
      expectPricingRow(pricingFile, rows, display, [
        `${cents(addon.priceCents)}/mo`,
        addon.availableOn.map(planLabel).join(", "),
        "Sellable",
      ]);
      continue;
    }
    expectPricingRow(pricingFile, rows, addon.displayName, [
      "Roadmap",
      "Contact sales",
      "Not sellable",
    ]);
  }

  const enterpriseRoadmap = proseList(enterprise.roadmapFeatures);
  expectText(
    pricingFile,
    pricingText,
    enterpriseRoadmap,
    "Enterprise roadmap feature list from catalog"
  );
  expectText(
    faqFile,
    faqText,
    enterpriseRoadmap,
    "Enterprise roadmap feature list from catalog"
  );

  for (const [file, text] of [
    [pricingFile, pricingText],
    [faqFile, faqText],
  ]) {
    if (text.toLowerCase().includes("workflow run")) {
      err(
        file,
        1,
        "billing docs must use 'orchestration run', not 'workflow run'"
      );
    }
    expectText(
      file,
      text,
      "orchestration run",
      "orchestration-run billing unit"
    );
  }

  for (const event of [
    "billing.cap_warning",
    "billing.cap_reached",
    "billing.cap_disabled",
    "billing.overage_disabled",
    "billing.suspended",
    "billing.delinquent",
    "schedule.suspended",
    "workflow.registration_rejected",
    "sla.credit_issued",
  ]) {
    expectText(
      webhookFile,
      webhookText,
      `\`${event}\``,
      "launch billing webhook event"
    );
  }

  for (const suffix of [
    "cap_warning",
    "cap_reached",
    "cap_disabled",
    "overage_disabled",
    "suspended",
    "delinquent",
  ]) {
    const staleEvent = ["subscription", suffix].join(".");
    if (webhookText.includes(staleEvent)) {
      err(
        webhookFile,
        1,
        `webhook docs must use billing.* event names, found '${staleEvent}'`
      );
    }
  }
}

checkBillingCatalogDocs();
checkDocumentedHttpMethods();
checkEnvCoverage();
checkRunStateDocs();
checkWebhookEventDocs();
checkFirstPartyMarkdownLinks();
checkReadmeCommands();
checkAgentDocsSync();

// ---- report ----------------------------------------------------------------
if (errors.length === 0) {
  console.log(
    `docs-lint: ${files.length} mdx files and ${firstPartyMarkdownFiles.length} markdown files checked, no problems found`
  );
  process.exit(0);
}
errors.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line);
for (const e of errors) {
  console.error(`${e.file}:${e.line}  ${e.msg}`);
}
console.error(
  `\ndocs-lint: ${errors.length} problem(s) in ${files.length} mdx files and ${firstPartyMarkdownFiles.length} markdown files`
);
process.exit(1);
