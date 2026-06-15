#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const DOCS = join(dirname(fileURLToPath(import.meta.url)), "..");
const REPO = join(DOCS, "..", "..");
const STRAIT = join(REPO, "apps", "strait");
const OUTPUT = join(DOCS, "api-reference", "generated-fields.mdx");
const CHECK = process.argv.includes("--check");

const SECTIONS = [
  {
    title: "Jobs",
    operations: [
      ["Create job", "post", "/v1/jobs"],
      ["Trigger job", "post", "/v1/jobs/{jobID}/trigger"],
      ["Bulk trigger job", "post", "/v1/jobs/{jobID}/trigger/bulk"],
      ["Update job", "patch", "/v1/jobs/{jobID}"],
    ],
  },
  {
    title: "Runs",
    operations: [
      ["Bulk replay dead-letter runs", "post", "/v1/runs/bulk-dlq-replay"],
      ["Bulk replay runs", "post", "/v1/runs/bulk-replay"],
      ["Bulk cancel runs", "post", "/v1/runs/bulk-cancel"],
    ],
  },
  {
    title: "Webhooks",
    operations: [
      ["Test webhook", "post", "/v1/webhooks/test"],
      ["Create webhook subscription", "post", "/v1/webhooks/subscriptions"],
      [
        "Rotate webhook secret",
        "post",
        "/v1/webhooks/subscriptions/{id}/rotate-secret",
      ],
    ],
  },
  {
    title: "Event Sources",
    operations: [
      ["Create event source", "post", "/v1/event-sources"],
      ["Update event source", "patch", "/v1/event-sources/{sourceID}"],
      [
        "Subscribe target to source",
        "post",
        "/v1/event-sources/{sourceID}/subscribe",
      ],
      ["Dispatch event", "post", "/v1/events/dispatch"],
      ["Send event", "post", "/v1/events/{eventKey}/send"],
    ],
  },
  {
    title: "Projects and API Keys",
    operations: [
      ["Create project", "post", "/v1/projects"],
      ["Update project settings", "put", "/v1/projects/{projectID}/settings"],
      ["Create API key", "post", "/v1/api-keys"],
      ["Rotate API key", "post", "/v1/api-keys/{keyID}/rotate"],
    ],
  },
];

function runOpenAPIDump() {
  const dir = mkdtempSync(join(tmpdir(), "strait-openapi-"));
  const output = join(dir, "openapi.json");
  const result = spawnSync("go", ["run", "./scripts/dump-openapi", output], {
    cwd: STRAIT,
    encoding: "utf8",
  });
  if (result.status !== 0) {
    throw new Error(
      `failed to dump OpenAPI spec\nstdout:\n${result.stdout}\nstderr:\n${result.stderr}`
    );
  }
  const spec = JSON.parse(readFileSync(output, "utf8"));
  rmSync(dir, { recursive: true, force: true });
  return spec;
}

function schemaForOperation(spec, method, path) {
  const operation = spec.paths?.[path]?.[method];
  const schema = operation?.requestBody?.content?.["application/json"]?.schema;
  if (!schema) {
    return null;
  }
  return dereferenceSchema(spec, schema);
}

function dereferenceSchema(spec, schema) {
  if (!schema.$ref) {
    return schema;
  }
  const prefix = "#/components/schemas/";
  if (!schema.$ref.startsWith(prefix)) {
    throw new Error(`unsupported schema ref ${schema.$ref}`);
  }
  const name = schema.$ref.slice(prefix.length);
  const resolved = spec.components?.schemas?.[name];
  if (!resolved) {
    throw new Error(`missing schema component ${name}`);
  }
  return resolved;
}

function typeOf(schema) {
  if (!schema) {
    return "any";
  }
  if (schema.$ref) {
    return schema.$ref.split("/").at(-1);
  }
  if (Array.isArray(schema.type)) {
    return schema.type.filter((value) => value !== "null").join(" or ");
  }
  if (schema.type === "array") {
    return `${typeOf(schema.items)}[]`;
  }
  if (schema.type === "object" && schema.additionalProperties) {
    return `object<string, ${typeOf(schema.additionalProperties)}>`;
  }
  return schema.type ?? "any";
}

function escapeCell(value) {
  return String(value || "")
    .replaceAll("|", "\\|")
    .replaceAll("\n", " ")
    .trim();
}

function fieldsTable(schema) {
  const required = new Set(schema.required ?? []);
  const properties = Object.entries(schema.properties ?? {}).filter(
    ([name]) => name !== "$schema"
  );
  if (properties.length === 0) {
    return "This request body has no documented JSON fields.\n";
  }
  const rows = [
    "| Field | Type | Required | Description |",
    "|---|---|---|---|",
  ];
  for (const [name, prop] of properties) {
    rows.push(
      `| \`${name}\` | ${escapeCell(typeOf(prop))} | ${
        required.has(name) ? "Yes" : "No"
      } | ${escapeCell(prop.description)} |`
    );
  }
  return `${rows.join("\n")}\n`;
}

function render(spec) {
  const lines = [
    "---",
    'title: "Generated API Fields"',
    'description: "Request body field tables generated from the runtime OpenAPI schema."',
    'icon: "table-properties"',
    "---",
    "",
    "This page is generated from the runtime Huma OpenAPI schema.",
    "",
    "Do not edit this page by hand. Run:",
    "",
    "```bash",
    "cd apps/docs",
    "bun run generate:api-fields",
    "```",
    "",
  ];

  for (const section of SECTIONS) {
    lines.push(`## ${section.title}`, "");
    for (const [label, method, path] of section.operations) {
      const schema = schemaForOperation(spec, method, path);
      if (!schema) {
        lines.push(
          `### ${label}`,
          "",
          `\`${method.toUpperCase()} ${path}\``,
          ""
        );
        lines.push("This operation has no JSON request body.", "");
        continue;
      }
      lines.push(`### ${label}`, "", `\`${method.toUpperCase()} ${path}\``, "");
      lines.push(fieldsTable(schema), "");
    }
  }

  return `${lines.join("\n").replace(/\n{3,}/g, "\n\n")}\n`;
}

const generated = render(runOpenAPIDump());
const current = existsSync(OUTPUT) ? readFileSync(OUTPUT, "utf8") : "";

if (CHECK) {
  if (generated !== current) {
    console.error(
      "generated API field tables are stale; run `bun run generate:api-fields`"
    );
    process.exit(1);
  }
  console.log("api-field-tables: generated page is current");
  process.exit(0);
}

writeFileSync(OUTPUT, generated);
console.log(`wrote ${OUTPUT}`);
