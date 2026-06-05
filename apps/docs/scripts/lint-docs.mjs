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

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
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

// ---- report ----------------------------------------------------------------
if (errors.length === 0) {
  console.log(`docs-lint: ${files.length} files checked, no problems found`);
  process.exit(0);
}
errors.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line);
for (const e of errors) {
  console.error(`${e.file}:${e.line}  ${e.msg}`);
}
console.error(
  `\ndocs-lint: ${errors.length} problem(s) in ${files.length} files`
);
process.exit(1);
