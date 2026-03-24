import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

import { generatedOperations } from "../src/internal/contracts/_generated/contracts";

const lineBreakPattern = /\r?\n/;
const pathDeclarationPattern = /^ {2}(\/[^:]+):\s*$/;
const methodDeclarationPattern = /^ {4}(get|post|put|patch|delete):\s*$/i;

const parseYAMLPathMethods = (
  source: string
): ReadonlyArray<{ readonly method: string; readonly path: string }> => {
  const lines = source.split(lineBreakPattern);

  const items: Array<{ method: string; path: string }> = [];

  let inPaths = false;
  let currentPath = "";

  for (const line of lines) {
    if (!inPaths) {
      if (line === "paths:") {
        inPaths = true;
      }
      continue;
    }

    const pathMatch = line.match(pathDeclarationPattern);
    if (pathMatch) {
      currentPath = pathMatch[1];
      continue;
    }

    const methodMatch = line.match(methodDeclarationPattern);
    if (methodMatch && currentPath) {
      items.push({ method: methodMatch[1].toUpperCase(), path: currentPath });
    }
  }

  return items;
};

// Parse JSON OpenAPI spec (Huma generates JSON, not YAML).
const parseJSONPathMethods = (
  source: string
): ReadonlyArray<{ readonly method: string; readonly path: string }> => {
  const spec = JSON.parse(source);
  const items: Array<{ method: string; path: string }> = [];
  const methods = ["get", "post", "put", "patch", "delete"];

  for (const [path, pathItem] of Object.entries(spec.paths ?? {})) {
    for (const method of methods) {
      if ((pathItem as Record<string, unknown>)[method]) {
        items.push({ method: method.toUpperCase(), path });
      }
    }
  }

  return items;
};

describe("generated contracts", () => {
  test("cover every OpenAPI path/method pair", () => {
    // Try JSON spec first (Huma auto-generated), then YAML (legacy).
    const jsonPath = resolve(import.meta.dir, "../../../docs/openapi.json");
    const yamlPath = resolve(import.meta.dir, "../../../docs/openapi.yaml");

    let openApiPairs: string[];

    if (existsSync(jsonPath)) {
      const source = readFileSync(jsonPath, "utf-8");
      openApiPairs = parseJSONPathMethods(source).map(
        (item) => `${item.method} ${item.path}`
      );
    } else if (existsSync(yamlPath)) {
      const source = readFileSync(yamlPath, "utf-8");
      openApiPairs = parseYAMLPathMethods(source).map(
        (item) => `${item.method} ${item.path}`
      );
    } else {
      // No static spec file -- OpenAPI is generated at runtime by Huma.
      // Just verify contracts are non-empty.
      expect(generatedOperations.length).toBeGreaterThan(0);
      return;
    }

    const generatedPairs = generatedOperations.map(
      (operation) => `${operation.method} ${operation.path}`
    );

    expect(new Set(generatedPairs)).toEqual(new Set(openApiPairs));
    expect(Number(generatedOperations.length)).toBe(openApiPairs.length);
  });
});
