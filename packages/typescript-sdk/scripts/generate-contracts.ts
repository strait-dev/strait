import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { parseDocument } from "yaml";

type Method = "DELETE" | "GET" | "PATCH" | "POST" | "PUT";

type Json = null | boolean | number | string | Json[] | { [key: string]: Json };

type OpenApiSchema = {
  readonly $ref?: string;
  readonly type?: string;
  readonly enum?: readonly Json[];
  readonly oneOf?: readonly OpenApiSchema[];
  readonly anyOf?: readonly OpenApiSchema[];
  readonly allOf?: readonly OpenApiSchema[];
  readonly items?: OpenApiSchema;
  readonly properties?: Readonly<Record<string, OpenApiSchema>>;
  readonly required?: readonly string[];
  readonly nullable?: boolean;
  readonly additionalProperties?: boolean | OpenApiSchema;
};

type OpenApiMedia = {
  readonly schema?: OpenApiSchema;
};

type OpenApiRequestBody = {
  readonly $ref?: string;
  readonly content?: Readonly<Record<string, OpenApiMedia>>;
};

type OpenApiResponse = {
  readonly $ref?: string;
  readonly content?: Readonly<Record<string, OpenApiMedia>>;
};

type OpenApiOperation = {
  readonly tags?: readonly string[];
  readonly summary?: string;
  readonly requestBody?: OpenApiRequestBody;
  readonly responses?: Readonly<Record<string, OpenApiResponse>>;
};

type OpenApiPathItem = Readonly<
  Partial<Record<Lowercase<Method>, OpenApiOperation>>
>;

type OpenApiDocument = {
  readonly paths?: Readonly<Record<string, OpenApiPathItem>>;
  readonly components?: {
    readonly schemas?: Readonly<Record<string, OpenApiSchema>>;
    readonly requestBodies?: Readonly<Record<string, OpenApiRequestBody>>;
    readonly responses?: Readonly<Record<string, OpenApiResponse>>;
  };
};

type ParsedOperation = {
  readonly id: string;
  readonly tag: string;
  readonly method: Method;
  readonly path: string;
  readonly summary?: string;
  readonly requestSchemaExpr?: string;
  readonly responseSchemaExpr?: string;
};

const methodOrder: Record<Method, number> = {
  DELETE: 0,
  GET: 1,
  PATCH: 2,
  POST: 3,
  PUT: 4,
};

const supportedMethods = ["delete", "get", "patch", "post", "put"] as const;
const nonAlphaNumericPattern = /[^a-zA-Z0-9]+/g;
const whitespacePattern = /\s+/;
const refPrefixPattern = /^#\//;
const successStatusPattern = /^2\d\d$/;

const toMethod = (value: string): Method => value.toUpperCase() as Method;

const sanitizeToken = (value: string): string =>
  value
    .replaceAll("{", "")
    .replaceAll("}", "")
    .replaceAll(nonAlphaNumericPattern, " ")
    .trim()
    .split(whitespacePattern)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join("");

const operationIdFrom = (method: Method, path: string): string => {
  const parts = path
    .split("/")
    .filter(Boolean)
    .map((segment) => {
      if (segment.startsWith("{") && segment.endsWith("}")) {
        return `By${sanitizeToken(segment)}`;
      }

      return sanitizeToken(segment);
    });

  return `${method.toLowerCase()}${parts.join("")}`;
};

const isObjectRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const resolveRef = (doc: OpenApiDocument, ref: string): unknown => {
  const refPath = ref.replace(refPrefixPattern, "").split("/");

  let current: unknown = doc;
  for (const token of refPath) {
    if (!(isObjectRecord(current) && token in current)) {
      return undefined;
    }

    current = current[token];
  }

  return current;
};

const unwrapRequestBody = (
  doc: OpenApiDocument,
  requestBody: OpenApiRequestBody | undefined
): OpenApiRequestBody | undefined => {
  if (!requestBody) {
    return undefined;
  }

  if (requestBody.$ref) {
    const resolved = resolveRef(doc, requestBody.$ref);
    return isObjectRecord(resolved)
      ? (resolved as OpenApiRequestBody)
      : undefined;
  }

  return requestBody;
};

const unwrapResponse = (
  doc: OpenApiDocument,
  response: OpenApiResponse | undefined
): OpenApiResponse | undefined => {
  if (!response) {
    return undefined;
  }

  if (response.$ref) {
    const resolved = resolveRef(doc, response.$ref);
    return isObjectRecord(resolved) ? (resolved as OpenApiResponse) : undefined;
  }

  return response;
};

const resolveSchemaObject = (
  doc: OpenApiDocument,
  schema: OpenApiSchema | undefined
): OpenApiSchema | undefined => {
  if (!schema) {
    return undefined;
  }

  if (schema.$ref) {
    const resolved = resolveRef(doc, schema.$ref);
    return isObjectRecord(resolved) ? (resolved as OpenApiSchema) : undefined;
  }

  return schema;
};

const getJsonSchemaFromMedia = (
  content: Readonly<Record<string, OpenApiMedia>> | undefined
): OpenApiSchema | undefined => {
  if (!content) {
    return undefined;
  }

  if (content["application/json"]?.schema) {
    return content["application/json"]?.schema;
  }

  for (const [contentType, media] of Object.entries(content)) {
    if (contentType.includes("json") && media.schema) {
      return media.schema;
    }
  }

  return undefined;
};

const schemaExprFromRef = (ref: string): string => {
  const componentPrefix = "#/components/schemas/";
  if (ref.startsWith(componentPrefix)) {
    const schemaName = ref.slice(componentPrefix.length);
    return `Schema.suspend(() => componentSchemas[${JSON.stringify(schemaName)}] ?? Schema.Unknown)`;
  }

  return "Schema.Unknown";
};

const scalarEnumExpr = (values: readonly Json[]): string | undefined => {
  const scalarEnum = values.filter((value) => {
    const valueType = typeof value;
    return (
      valueType === "string" ||
      valueType === "number" ||
      valueType === "boolean" ||
      value === null
    );
  });

  if (scalarEnum.length === 0) {
    return undefined;
  }

  return `Schema.Literal(${scalarEnum.map((value) => JSON.stringify(value)).join(", ")})`;
};

const objectSchemaExpr = (
  doc: OpenApiDocument,
  schema: OpenApiSchema
): string => {
  const properties = schema.properties ?? {};
  const required = new Set(schema.required ?? []);

  if (Object.keys(properties).length > 0) {
    const fields = Object.entries(properties).map(
      ([propertyName, propertySchema]) => {
        const propertyExpr = schemaExpr(doc, propertySchema);
        const expr = required.has(propertyName)
          ? propertyExpr
          : `Schema.optional(${propertyExpr})`;
        return `${JSON.stringify(propertyName)}: ${expr}`;
      }
    );

    return `Schema.Struct({ ${fields.join(", ")} })`;
  }

  if (
    schema.additionalProperties &&
    typeof schema.additionalProperties === "object"
  ) {
    return `Schema.Record({ key: Schema.String, value: ${schemaExpr(doc, schema.additionalProperties)} })`;
  }

  return "Schema.Record({ key: Schema.String, value: Schema.Unknown })";
};

const primitiveSchemaExpr = (
  baseType: string | undefined
): string | undefined => {
  switch (baseType) {
    case "array":
      return undefined;
    case "boolean":
      return "Schema.Boolean";
    case "integer":
      return "Schema.Number.pipe(Schema.int())";
    case "number":
      return "Schema.Number";
    case "string":
      return "Schema.String";
    default:
      return undefined;
  }
};

const schemaExpr = (
  doc: OpenApiDocument,
  schema: OpenApiSchema | undefined
): string => {
  if (!schema) {
    return "Schema.Unknown";
  }

  if (schema.$ref) {
    return schemaExprFromRef(schema.$ref);
  }

  const unionSchemas = schema.oneOf ?? schema.anyOf;
  if (unionSchemas && unionSchemas.length > 0) {
    return `Schema.Union(${unionSchemas.map((item) => schemaExpr(doc, item)).join(", ")})`;
  }

  if (schema.allOf && schema.allOf.length > 0) {
    return "Schema.Unknown";
  }

  const enumExpr = schema.enum ? scalarEnumExpr(schema.enum) : undefined;
  if (enumExpr) {
    return enumExpr;
  }

  let baseExpr: string;
  if (schema.type === "array") {
    baseExpr = `Schema.Array(${schemaExpr(doc, schema.items)})`;
  } else {
    const primitiveExpr = primitiveSchemaExpr(schema.type);
    if (primitiveExpr) {
      baseExpr = primitiveExpr;
    } else if (
      schema.type === "object" ||
      schema.properties ||
      schema.additionalProperties !== undefined
    ) {
      baseExpr = objectSchemaExpr(doc, schema);
    } else {
      baseExpr = "Schema.Unknown";
    }
  }

  return schema.nullable ? `Schema.NullOr(${baseExpr})` : baseExpr;
};

const pickRequestSchemaExpr = (
  doc: OpenApiDocument,
  operation: OpenApiOperation
): string | undefined => {
  const requestBody = unwrapRequestBody(doc, operation.requestBody);
  const schema = getJsonSchemaFromMedia(requestBody?.content);
  const resolved = resolveSchemaObject(doc, schema);

  if (!resolved) {
    return undefined;
  }

  return schemaExpr(doc, schema);
};

const pickResponseSchemaExpr = (
  doc: OpenApiDocument,
  operation: OpenApiOperation
): string | undefined => {
  const responses = operation.responses;
  if (!responses) {
    return undefined;
  }

  const successStatusCode = Object.keys(responses)
    .filter((code) => successStatusPattern.test(code))
    .sort((a, b) => Number(a) - Number(b))[0];

  if (!successStatusCode) {
    return undefined;
  }

  const response = unwrapResponse(doc, responses[successStatusCode]);
  const schema = getJsonSchemaFromMedia(response?.content);
  const resolved = resolveSchemaObject(doc, schema);

  if (!resolved) {
    return undefined;
  }

  return schemaExpr(doc, schema);
};

const parseOpenApiOperations = (doc: OpenApiDocument): ParsedOperation[] => {
  const operations: ParsedOperation[] = [];

  for (const [path, pathItem] of Object.entries(doc.paths ?? {})) {
    for (const methodKey of supportedMethods) {
      const operation = pathItem[methodKey];
      if (!operation) {
        continue;
      }

      const method = toMethod(methodKey);
      operations.push({
        id: operationIdFrom(method, path),
        tag: operation.tags?.[0] ?? "Uncategorized",
        method,
        path,
        summary: operation.summary,
        requestSchemaExpr: pickRequestSchemaExpr(doc, operation),
        responseSchemaExpr: pickResponseSchemaExpr(doc, operation),
      });
    }
  }

  return operations.sort((a, b) => {
    if (a.path !== b.path) {
      return a.path.localeCompare(b.path);
    }

    return methodOrder[a.method] - methodOrder[b.method];
  });
};

const renderOperationsFile = (
  operations: readonly ParsedOperation[]
): string => {
  const operationsLiteral = operations
    .map((operation) => {
      const summaryField = operation.summary
        ? `, summary: ${JSON.stringify(operation.summary)}`
        : "";
      return `  { id: ${JSON.stringify(operation.id)}, tag: ${JSON.stringify(operation.tag)}, method: ${JSON.stringify(operation.method)}, path: ${JSON.stringify(operation.path)}${summaryField} },`;
    })
    .join("\n");

  return `/* eslint-disable */
// This file is generated by scripts/generate-contracts.ts

export type GeneratedOperation = {
  readonly id: string;
  readonly tag: string;
  readonly method: "DELETE" | "GET" | "PATCH" | "POST" | "PUT";
  readonly path: string;
  readonly summary?: string;
};

export const generatedOperations = [
${operationsLiteral}
] as const satisfies ReadonlyArray<GeneratedOperation>;

export const generatedOperationMap = Object.fromEntries(
  generatedOperations.map((operation) => [operation.id, operation]),
) as Readonly<Record<string, GeneratedOperation>>;

export const generatedOperationsByTag = generatedOperations.reduce<Record<string, ReadonlyArray<GeneratedOperation>>>(
  (acc, operation) => {
    const current = acc[operation.tag] ?? [];
    acc[operation.tag] = [...current, operation];
    return acc;
  },
  {},
);
`;
};

const renderSchemaFile = (
  doc: OpenApiDocument,
  operations: readonly ParsedOperation[]
): string => {
  const componentAssignments = Object.entries(doc.components?.schemas ?? {})
    .map(
      ([name, schema]) =>
        `componentSchemas[${JSON.stringify(name)}] = ${schemaExpr(doc, schema)};`
    )
    .join("\n");

  const operationSchemaEntries = operations
    .map((operation) => {
      const fields: string[] = [];
      if (operation.requestSchemaExpr) {
        fields.push(`request: ${operation.requestSchemaExpr}`);
      }

      if (operation.responseSchemaExpr) {
        fields.push(`response: ${operation.responseSchemaExpr}`);
      }

      if (fields.length === 0) {
        return `  ${JSON.stringify(operation.id)}: {},`;
      }

      return `  ${JSON.stringify(operation.id)}: { ${fields.join(", ")} },`;
    })
    .join("\n");

  return `/* eslint-disable */
// This file is generated by scripts/generate-contracts.ts

import { Schema } from "effect";

export type GeneratedOperationSchema = {
  readonly request?: Schema.Schema.AnyNoContext;
  readonly response?: Schema.Schema.AnyNoContext;
};

const componentSchemas: Record<string, Schema.Schema.AnyNoContext> = {};
${componentAssignments}

export const generatedOperationSchemas: Readonly<Record<string, GeneratedOperationSchema>> = {
${operationSchemaEntries}
};
`;
};

const scriptDir = fileURLToPath(new URL(".", import.meta.url));
const packageRoot = resolve(scriptDir, "..");
const workspaceRoot = resolve(packageRoot, "..", "..");

const openApiPath = resolve(workspaceRoot, "docs/openapi.yaml");
const contractsDir = resolve(packageRoot, "src/internal/contracts");
const schemaDir = resolve(packageRoot, "src/internal/schema");

mkdirSync(contractsDir, { recursive: true });
mkdirSync(schemaDir, { recursive: true });
mkdirSync(resolve(contractsDir, "_generated"), { recursive: true });
mkdirSync(resolve(schemaDir, "_generated"), { recursive: true });

const source = readFileSync(openApiPath, "utf-8");
const openApiDocument = parseDocument(source, {
  uniqueKeys: false,
}).toJS() as OpenApiDocument;
const operations = parseOpenApiOperations(openApiDocument);

writeFileSync(
  resolve(contractsDir, "_generated/contracts.ts"),
  renderOperationsFile(operations),
  "utf-8"
);
writeFileSync(
  resolve(schemaDir, "_generated/schema.ts"),
  renderSchemaFile(openApiDocument, operations),
  "utf-8"
);

console.log(`Generated ${operations.length} operations plus schema wrappers`);
