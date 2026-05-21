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
