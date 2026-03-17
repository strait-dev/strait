import { describe, expect, test } from "bun:test";

import { generatedOperations } from "../src/internal/contracts/_generated/contracts";

const pathParamPattern = /\{([^}]+)\}/g;

const pathParamNamesFromTemplate = (path: string): string[] => {
  const names: string[] = [];
  for (const match of path.matchAll(pathParamPattern)) {
    names.push(match[1]);
  }

  return names;
};

describe("generated operation naming", () => {
  test("uses unique function names for top-level API generation", () => {
    const names = generatedOperations.map(
      (operation) => operation.functionName
    );
    expect(new Set(names).size).toBe(names.length);
  });

  test("uses unique domain method names per domain namespace", () => {
    const grouped = generatedOperations.reduce((acc, operation) => {
      const current = acc.get(operation.domainName) ?? [];
      current.push(operation.domainMethodName);
      acc.set(operation.domainName, current);
      return acc;
    }, new Map<string, string[]>());

    for (const [, domainMethods] of grouped.entries()) {
      expect(new Set(domainMethods).size).toBe(domainMethods.length);
    }
  });

  test("keeps generated path parameter names aligned with path templates", () => {
    for (const operation of generatedOperations) {
      const expected = pathParamNamesFromTemplate(operation.path);
      expect([...operation.pathParamNames] as string[]).toEqual(expected);
    }
  });
});
