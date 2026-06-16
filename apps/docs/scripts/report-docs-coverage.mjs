#!/usr/bin/env node

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const DOCS = join(dirname(fileURLToPath(import.meta.url)), "..");
const REPO = join(DOCS, "..", "..");
const API_DIR = join(DOCS, "api-reference");
const CONCEPT_DIR = join(DOCS, "concepts");
const GUIDES_DIR = join(DOCS, "guides");
const MDX_EXTENSION_RE = /\.mdx$/;
const WHITESPACE_RE = /\s+/;
const HUMA_REGISTRY_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "api", "huma_registry.go"),
  "utf8"
);
const HUMA_OPERATIONS_GO = readFileSync(
  join(REPO, "apps", "strait", "internal", "api", "huma_operations.go"),
  "utf8"
);

function walk(dir, extension, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) {
      walk(p, extension, out);
    } else if (name.endsWith(extension)) {
      out.push(p);
    }
  }
  return out;
}

function routeGroup(path) {
  if (path.startsWith("/v1/jobs")) {
    return "jobs";
  }
  if (path.startsWith("/v1/runs")) {
    return "runs";
  }
  if (
    path.startsWith("/v1/webhooks") ||
    path.startsWith("/v1/webhook-deliveries")
  ) {
    return "webhooks";
  }
  if (path.startsWith("/v1/event-sources") || path.startsWith("/v1/events")) {
    return "event-sources";
  }
  if (
    path.startsWith("/v1/projects") ||
    path.startsWith("/health") ||
    path.startsWith("/ready")
  ) {
    return "admin-ops";
  }
  if (path.startsWith("/v1/workers") || path.startsWith("/sdk/v1")) {
    return "workers";
  }
  return "";
}

function collectRoutes() {
  const re =
    /(?:ID|OperationID):\s*"([^"]+)"[\s\S]{0,160}?Method:\s*http\.Method(Get|Post|Put|Patch|Delete),\s*Path:\s*"([^"]+)"/g;
  const routes = [];
  for (const text of [HUMA_REGISTRY_GO, HUMA_OPERATIONS_GO]) {
    for (const match of text.matchAll(re)) {
      routes.push({
        id: match[1],
        method: match[2].toUpperCase(),
        path: match[3],
        group: routeGroup(match[3]),
      });
    }
  }
  return routes;
}

function mdxText(files) {
  return files.map((file) => readFileSync(file, "utf8")).join("\n");
}

const mdxFiles = walk(DOCS, ".mdx");
const apiText = mdxText(walk(API_DIR, ".mdx"));
const guideFiles = walk(GUIDES_DIR, ".mdx");
const routes = collectRoutes();
const documentedCuratedGroups = new Set(
  walk(API_DIR, ".mdx").map((file) =>
    relative(API_DIR, file).replace(MDX_EXTENSION_RE, "")
  )
);
const undocumentedRouteGroups = [
  ...new Set(
    routes
      .filter(
        (route) => route.group && !documentedCuratedGroups.has(route.group)
      )
      .map((route) => route.group)
  ),
].sort();
const routesNotMentionedInCuratedApi = routes
  .filter((route) => route.group)
  .filter((route) => !apiText.includes(`${route.method} ${route.path}`))
  .map((route) => `${route.method} ${route.path}`);
const guidesWithoutExamples = guideFiles
  .filter((file) => !readFileSync(file, "utf8").includes("```"))
  .map((file) => relative(DOCS, file));
const conceptPagesNotLinkedFromGuides = walk(CONCEPT_DIR, ".mdx")
  .map((file) => relative(DOCS, file).replace(MDX_EXTENSION_RE, ""))
  .filter((route) => !mdxText(guideFiles).includes(`/${route}`));
const thinPages = mdxFiles
  .map((file) => ({
    file: relative(DOCS, file),
    words: readFileSync(file, "utf8").split(WHITESPACE_RE).filter(Boolean)
      .length,
  }))
  .filter((entry) => entry.words < 120)
  .map((entry) => `${entry.file} (${entry.words} words)`);

const report = {
  counts: {
    mdxPages: mdxFiles.length,
    guides: guideFiles.length,
    concepts: walk(CONCEPT_DIR, ".mdx").length,
    apiRoutes: routes.length,
  },
  undocumentedRouteGroups,
  routesNotMentionedInCuratedApi,
  guidesWithoutExamples,
  conceptPagesNotLinkedFromGuides,
  thinPages,
};

console.log(JSON.stringify(report, null, 2));
