import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { parseDocument } from "yaml";

type Method = "DELETE" | "GET" | "PATCH" | "POST" | "PUT";
type ParameterLocation = "cookie" | "header" | "path" | "query";

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

type OpenApiParameter = {
  readonly $ref?: string;
  readonly name?: string;
  readonly in?: ParameterLocation;
  readonly required?: boolean;
  readonly schema?: OpenApiSchema;
};

type OpenApiOperation = {
  readonly tags?: readonly string[];
  readonly summary?: string;
  readonly requestBody?: OpenApiRequestBody;
  readonly responses?: Readonly<Record<string, OpenApiResponse>>;
  readonly parameters?: readonly OpenApiParameter[];
};

type OpenApiPathItem = Readonly<
  Partial<Record<Lowercase<Method>, OpenApiOperation>> & {
    readonly parameters?: readonly OpenApiParameter[];
  }
>;

type OpenApiDocument = {
  readonly paths?: Readonly<Record<string, OpenApiPathItem>>;
  readonly components?: {
    readonly schemas?: Readonly<Record<string, OpenApiSchema>>;
    readonly requestBodies?: Readonly<Record<string, OpenApiRequestBody>>;
    readonly responses?: Readonly<Record<string, OpenApiResponse>>;
    readonly parameters?: Readonly<Record<string, OpenApiParameter>>;
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
  readonly pathParamsTypeExpr: string;
  readonly queryTypeExpr: string;
  readonly headersTypeExpr: string;
  readonly pathParamNames: readonly string[];
  readonly functionName: string;
  readonly domainName: string;
  readonly domainMethodName: string;
};

type NamingDraft = {
  readonly action: string;
  readonly objectName: string;
  readonly operation: Omit<
    ParsedOperation,
    "functionName" | "domainName" | "domainMethodName"
  >;
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
const camelCaseBoundaryPattern = /([a-z0-9])([A-Z])/g;
const acronymBoundaryPattern = /([A-Z]+)([A-Z][a-z])/g;
const versionSegmentPattern = /^v\d+$/i;
const leadingSummaryFieldPrefixPattern = /^,\s*/;

const nonCollectionSegmentNames = new Set([
  "analytics",
  "health",
  "impact",
  "lineage",
  "metrics",
  "performance",
  "prefix",
  "ready",
  "stats",
  "status",
  "stream",
  "usage",
]);

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

const splitIdentifierWords = (value: string): readonly string[] =>
  value
    .replaceAll("{", "")
    .replaceAll("}", "")
    .replace(camelCaseBoundaryPattern, "$1 $2")
    .replace(acronymBoundaryPattern, "$1 $2")
    .replaceAll(nonAlphaNumericPattern, " ")
    .trim()
    .split(whitespacePattern)
    .filter(Boolean)
    .map((part) => part.toLowerCase());

const toPascalIdentifier = (value: string): string => {
  const words = splitIdentifierWords(value);
  if (words.length === 0) {
    return "Operation";
  }

  return words
    .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
    .join("");
};

const toCamelCase = (value: string): string => {
  const words = splitIdentifierWords(value);
  if (words.length === 0) {
    return "operation";
  }

  const [head, ...tail] = words;
  const tailPascal = tail
    .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
    .join("");

  return `${head}${tailPascal}`;
};

const singularizeToken = (value: string): string => {
  const words = [...splitIdentifierWords(value)];
  if (words.length === 0) {
    return "Item";
  }

  const lastIndex = words.length - 1;
  const last = words[lastIndex];
  if (last.endsWith("ies") && last.length > 3) {
    words[lastIndex] = `${last.slice(0, -3)}y`;
  } else if (last.endsWith("ses") && last.length > 3) {
    words[lastIndex] = last.slice(0, -2);
  } else if (last.endsWith("s") && last.length > 1 && !last.endsWith("ss")) {
    words[lastIndex] = last.slice(0, -1);
  }

  return words
    .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
    .join("");
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

const unwrapParameter = (
  doc: OpenApiDocument,
  parameter: OpenApiParameter
): OpenApiParameter | undefined => {
  if (parameter.$ref) {
    const resolved = resolveRef(doc, parameter.$ref);
    return isObjectRecord(resolved)
      ? (resolved as OpenApiParameter)
      : undefined;
  }

  return parameter;
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

const toNullableTypeExpr = (
  baseExpr: string,
  nullable: boolean | undefined,
  options?: { readonly wrapUnion?: boolean }
): string => {
  if (!nullable) {
    return baseExpr;
  }

  if (options?.wrapUnion) {
    return `(${baseExpr}) | null`;
  }

  return `${baseExpr} | null`;
};

const scalarLiteralTypeExpr = (
  values: readonly Json[] | undefined
): string | undefined => {
  if (!(values && values.length > 0)) {
    return undefined;
  }

  const literals = values
    .filter((value) => {
      const valueType = typeof value;
      return (
        valueType === "string" ||
        valueType === "number" ||
        valueType === "boolean" ||
        value === null
      );
    })
    .map((value) => JSON.stringify(value));

  if (literals.length === 0) {
    return undefined;
  }

  return literals.join(" | ");
};

const objectTypeExprFromSchema = (
  doc: OpenApiDocument,
  schema: OpenApiSchema
): string => {
  const properties = schema.properties ?? {};
  const required = new Set(schema.required ?? []);

  if (Object.keys(properties).length > 0) {
    const fields = Object.entries(properties)
      .map(([propertyName, propertySchema]) => {
        const optionalMarker = required.has(propertyName) ? "" : "?";
        return `${JSON.stringify(propertyName)}${optionalMarker}: ${tsTypeExprFromSchema(doc, propertySchema)}`;
      })
      .join("; ");

    return `{ ${fields} }`;
  }

  if (typeof schema.additionalProperties === "object") {
    return `Record<string, ${tsTypeExprFromSchema(doc, schema.additionalProperties)}>`;
  }

  return "Record<string, unknown>";
};

const baseTypeExprFromSchema = (
  doc: OpenApiDocument,
  schema: OpenApiSchema
): string => {
  if (schema.type === "boolean") {
    return "boolean";
  }

  if (schema.type === "integer" || schema.type === "number") {
    return "number";
  }

  if (schema.type === "string") {
    return "string";
  }

  if (schema.type === "array") {
    return `Array<${tsTypeExprFromSchema(doc, schema.items)}>`;
  }

  if (
    schema.type === "object" ||
    schema.properties ||
    schema.additionalProperties !== undefined
  ) {
    return objectTypeExprFromSchema(doc, schema);
  }

  return "unknown";
};

const tsTypeExprFromSchema = (
  doc: OpenApiDocument,
  schema: OpenApiSchema | undefined
): string => {
  if (!schema) {
    return "unknown";
  }

  const resolved = resolveSchemaObject(doc, schema);
  if (!resolved) {
    return "unknown";
  }

  const literalTypeExpr = scalarLiteralTypeExpr(resolved.enum);
  if (literalTypeExpr) {
    return toNullableTypeExpr(literalTypeExpr, resolved.nullable, {
      wrapUnion: true,
    });
  }

  const unionSchemas = resolved.oneOf ?? resolved.anyOf;
  if (unionSchemas && unionSchemas.length > 0) {
    const unionTypeExpr = unionSchemas
      .map((item) => tsTypeExprFromSchema(doc, item))
      .join(" | ");
    return toNullableTypeExpr(unionTypeExpr, resolved.nullable, {
      wrapUnion: true,
    });
  }

  if (resolved.allOf && resolved.allOf.length > 0) {
    const intersectionTypeExpr = resolved.allOf
      .map((item) => tsTypeExprFromSchema(doc, item))
      .join(" & ");
    return toNullableTypeExpr(intersectionTypeExpr, resolved.nullable, {
      wrapUnion: true,
    });
  }

  return toNullableTypeExpr(
    baseTypeExprFromSchema(doc, resolved),
    resolved.nullable
  );
};

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

const toPathSegments = (path: string): readonly string[] =>
  path
    .split("/")
    .filter(Boolean)
    .filter((segment) => !versionSegmentPattern.test(segment));

const isPathParamSegment = (segment: string): boolean =>
  segment.startsWith("{") && segment.endsWith("}");

const toDomainName = (tag: string): string => toCamelCase(tag);

const isCollectionSegment = (segment: string): boolean => {
  const lower = segment.toLowerCase();
  if (nonCollectionSegmentNames.has(lower)) {
    return false;
  }

  return lower.endsWith("s") && !lower.endsWith("ss");
};

const toActionFromPostSegment = (segment: string): string => {
  const token = sanitizeToken(segment);
  if (token.length === 0) {
    return "post";
  }

  return `${token.charAt(0).toLowerCase()}${token.slice(1)}`;
};

const inferGetNameParts = (
  primarySegment: string,
  lastSegment: string,
  hasPathParams: boolean,
  pathSegments: readonly string[],
  nonParamSegments: readonly string[]
): { readonly action: string; readonly objectSegment: string } => {
  if (!hasPathParams && lastSegment === primarySegment) {
    return { action: "list", objectSegment: primarySegment };
  }

  if (hasPathParams && isPathParamSegment(pathSegments.at(-1) ?? "")) {
    return {
      action: "get",
      objectSegment: nonParamSegments.at(-1) ?? primarySegment,
    };
  }

  return {
    action: isCollectionSegment(lastSegment) ? "list" : "get",
    objectSegment: lastSegment,
  };
};

const inferPostNameParts = (
  primarySegment: string,
  lastSegment: string,
  hasPathParams: boolean,
  nonParamSegments: readonly string[]
): { readonly action: string; readonly objectSegment: string } => {
  if (!hasPathParams && lastSegment === primarySegment) {
    return { action: "create", objectSegment: primarySegment };
  }

  if (lastSegment === primarySegment) {
    return { action: "create", objectSegment: primarySegment };
  }

  return {
    action: toActionFromPostSegment(lastSegment),
    objectSegment: nonParamSegments.at(-2) ?? primarySegment,
  };
};

const inferMethodNameParts = (
  method: Method,
  primarySegment: string,
  lastSegment: string,
  hasPathParams: boolean,
  pathSegments: readonly string[],
  nonParamSegments: readonly string[]
): { readonly action: string; readonly objectSegment: string } => {
  switch (method) {
    case "GET":
      return inferGetNameParts(
        primarySegment,
        lastSegment,
        hasPathParams,
        pathSegments,
        nonParamSegments
      );
    case "POST":
      return inferPostNameParts(
        primarySegment,
        lastSegment,
        hasPathParams,
        nonParamSegments
      );
    case "PATCH":
      return {
        action: "update",
        objectSegment: nonParamSegments.at(-1) ?? primarySegment,
      };
    case "PUT":
      return {
        action: "upsert",
        objectSegment: nonParamSegments.at(-1) ?? primarySegment,
      };
    case "DELETE":
      return {
        action: "delete",
        objectSegment: nonParamSegments.at(-1) ?? primarySegment,
      };
    default:
      return {
        action: "execute",
        objectSegment: primarySegment,
      };
  }
};

const inferNamingDraft = (
  operation: Omit<
    ParsedOperation,
    "functionName" | "domainName" | "domainMethodName"
  >
): NamingDraft => {
  const pathSegments = toPathSegments(operation.path);
  const nonParamSegments = pathSegments.filter(
    (segment) => !isPathParamSegment(segment)
  );

  const primarySegment = nonParamSegments[0] ?? "operation";
  const lastSegment = nonParamSegments.at(-1) ?? primarySegment;
  const hasPathParams = operation.pathParamNames.length > 0;

  const nameParts = inferMethodNameParts(
    operation.method,
    primarySegment,
    lastSegment,
    hasPathParams,
    pathSegments,
    nonParamSegments
  );

  const objectName =
    nameParts.action === "list"
      ? sanitizeToken(nameParts.objectSegment)
      : singularizeToken(nameParts.objectSegment);

  return {
    action: nameParts.action,
    objectName: objectName.length > 0 ? objectName : "Operation",
    operation,
  };
};

const buildNameCandidates = (entry: NamingDraft): string[] => {
  const baseName = `${entry.action}${entry.objectName}`;
  const candidates: string[] = [baseName];

  if (entry.operation.pathParamNames.length > 0) {
    const bySuffix = entry.operation.pathParamNames
      .map((paramName) => sanitizeToken(paramName))
      .filter(Boolean)
      .join("And");
    if (bySuffix.length > 0) {
      candidates.push(`${baseName}By${bySuffix}`);
    }
  }

  candidates.push(
    `${baseName}${sanitizeToken(entry.operation.method.toLowerCase())}`
  );
  candidates.push(
    `${baseName}${sanitizeToken(entry.operation.id.slice(0, 1).toUpperCase() + entry.operation.id.slice(1))}`
  );

  return candidates;
};

const pickFirstAvailableName = (
  candidates: readonly string[],
  usedNames: ReadonlySet<string>
): string | undefined => {
  for (const candidate of candidates) {
    const sanitized = candidate.replace(nonAlphaNumericPattern, "");
    if (!usedNames.has(sanitized) && sanitized.length > 0) {
      return sanitized;
    }
  }

  return undefined;
};

const pickFallbackName = (
  baseName: string,
  usedNames: ReadonlySet<string>
): string => {
  let counter = 2;
  for (;;) {
    const fallback = `${baseName}${counter}`;
    if (!usedNames.has(fallback)) {
      return fallback;
    }
    counter += 1;
  }
};

const ensureUniqueName = (
  entries: readonly NamingDraft[],
  existingNames?: ReadonlySet<string>
): ReadonlyMap<string, string> => {
  const usedNames = new Set(existingNames ?? []);
  const result = new Map<string, string>();

  for (const entry of entries) {
    const candidates = buildNameCandidates(entry);
    const baseName = `${entry.action}${entry.objectName}`;
    const assigned =
      pickFirstAvailableName(candidates, usedNames) ??
      pickFallbackName(baseName, usedNames);

    usedNames.add(assigned);
    result.set(entry.operation.id, assigned);
  }

  return result;
};

const normalizePublicFunctionName = (name: string): string => {
  if (name.length === 0) {
    return "operation";
  }

  return `${name.charAt(0).toLowerCase()}${name.slice(1)}`;
};

const deriveDomainMethodSeed = (
  globalName: string,
  draft: NamingDraft,
  tag: string
): string => {
  const domainResourceSingular = singularizeToken(tag);
  const domainResourcePlural = toPascalIdentifier(tag);
  const nameWithoutActionPrefix = globalName.slice(draft.action.length);

  const stripDomainPrefix = (prefix: string): string | undefined => {
    if (!nameWithoutActionPrefix.startsWith(prefix)) {
      return undefined;
    }

    const suffix = nameWithoutActionPrefix.slice(prefix.length);
    if (suffix.length === 0) {
      return draft.action;
    }

    return `${draft.action}${suffix}`;
  };

  return (
    stripDomainPrefix(domainResourcePlural) ??
    stripDomainPrefix(domainResourceSingular) ??
    globalName
  );
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

const mergeOperationParameters = (
  doc: OpenApiDocument,
  pathItem: OpenApiPathItem,
  operation: OpenApiOperation
): readonly OpenApiParameter[] => {
  const merged = new Map<string, OpenApiParameter>();
  const register = (parameter: OpenApiParameter): void => {
    const unwrapped = unwrapParameter(doc, parameter);
    if (!(unwrapped?.name && unwrapped.in)) {
      return;
    }

    const key = `${unwrapped.in}:${unwrapped.name}`;
    merged.set(key, unwrapped);
  };

  for (const parameter of pathItem.parameters ?? []) {
    register(parameter);
  }

  for (const parameter of operation.parameters ?? []) {
    register(parameter);
  }

  return [...merged.values()];
};

const renderParameterObjectType = (
  doc: OpenApiDocument,
  parameters: readonly OpenApiParameter[],
  location: Exclude<ParameterLocation, "cookie">
): { readonly typeExpr: string; readonly names: readonly string[] } => {
  const scopedParameters = parameters.filter(
    (parameter): parameter is OpenApiParameter & { readonly name: string } =>
      parameter.in === location && typeof parameter.name === "string"
  );

  if (scopedParameters.length === 0) {
    return { typeExpr: "undefined", names: [] };
  }

  const fields = scopedParameters
    .map((parameter) => {
      const required = location === "path" ? true : parameter.required === true;
      const optionalMarker = required ? "" : "?";
      const valueExpr = tsTypeExprFromSchema(doc, parameter.schema);
      return `${JSON.stringify(parameter.name)}${optionalMarker}: ${valueExpr}`;
    })
    .join("; ");

  return {
    typeExpr: `{ ${fields} }`,
    names: scopedParameters.map((parameter) => parameter.name),
  };
};

const parseOpenApiOperations = (doc: OpenApiDocument): ParsedOperation[] => {
  const drafts: Omit<
    ParsedOperation,
    "functionName" | "domainName" | "domainMethodName"
  >[] = [];

  for (const [path, pathItem] of Object.entries(doc.paths ?? {})) {
    for (const methodKey of supportedMethods) {
      const operation = pathItem[methodKey];
      if (!operation) {
        continue;
      }

      const method = toMethod(methodKey);
      const parameters = mergeOperationParameters(doc, pathItem, operation);
      const pathParams = renderParameterObjectType(doc, parameters, "path");
      const queryParams = renderParameterObjectType(doc, parameters, "query");
      const headerParams = renderParameterObjectType(doc, parameters, "header");

      drafts.push({
        id: operationIdFrom(method, path),
        tag: operation.tags?.[0] ?? "Uncategorized",
        method,
        path,
        summary: operation.summary,
        requestSchemaExpr: pickRequestSchemaExpr(doc, operation),
        responseSchemaExpr: pickResponseSchemaExpr(doc, operation),
        pathParamsTypeExpr: pathParams.typeExpr,
        queryTypeExpr: queryParams.typeExpr,
        headersTypeExpr: headerParams.typeExpr,
        pathParamNames: pathParams.names,
      });
    }
  }

  const sorted = drafts.sort((a, b) => {
    if (a.path !== b.path) {
      return a.path.localeCompare(b.path);
    }

    if (a.method !== b.method) {
      return methodOrder[a.method] - methodOrder[b.method];
    }

    return a.id.localeCompare(b.id);
  });

  const namingOperations = [...sorted].sort((a, b) => {
    const aDepth = toPathSegments(a.path).length;
    const bDepth = toPathSegments(b.path).length;
    if (aDepth !== bDepth) {
      return aDepth - bDepth;
    }

    if (a.path !== b.path) {
      return a.path.localeCompare(b.path);
    }

    return methodOrder[a.method] - methodOrder[b.method];
  });

  const namingDrafts = namingOperations.map((operation) =>
    inferNamingDraft(operation)
  );
  const globalNames = ensureUniqueName(namingDrafts);

  const byDomain = new Map<string, NamingDraft[]>();
  for (const draft of namingDrafts) {
    const key = draft.operation.tag;
    const current = byDomain.get(key) ?? [];
    current.push(draft);
    byDomain.set(key, current);
  }

  const domainMethodNames = new Map<string, string>();

  for (const [tag, items] of byDomain.entries()) {
    const seeds: NamingDraft[] = items.map((item) => {
      const globalName =
        globalNames.get(item.operation.id) ??
        `${item.action}${item.objectName}`;
      const domainSeed = deriveDomainMethodSeed(globalName, item, tag);

      return {
        ...item,
        action: toCamelCase(domainSeed),
        objectName: "",
      };
    });

    const uniqueDomainNames = ensureUniqueName(seeds);
    for (const item of items) {
      const resolved =
        uniqueDomainNames.get(item.operation.id) ??
        globalNames.get(item.operation.id) ??
        item.operation.id;
      domainMethodNames.set(
        item.operation.id,
        normalizePublicFunctionName(resolved)
      );
    }
  }

  return sorted.map((operation) => {
    const globalName = normalizePublicFunctionName(
      globalNames.get(operation.id) ?? operation.id
    );
    const domainName = toDomainName(operation.tag);

    return {
      ...operation,
      functionName: globalName,
      domainName,
      domainMethodName:
        domainMethodNames.get(operation.id) ??
        normalizePublicFunctionName(globalName),
    };
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

      const tokens = [
        `id: ${JSON.stringify(operation.id)}`,
        `tag: ${JSON.stringify(operation.tag)}`,
        `method: ${JSON.stringify(operation.method)}`,
        `path: ${JSON.stringify(operation.path)}`,
        `functionName: ${JSON.stringify(operation.functionName)}`,
        `domainName: ${JSON.stringify(operation.domainName)}`,
        `domainMethodName: ${JSON.stringify(operation.domainMethodName)}`,
        `pathParamNames: ${JSON.stringify(operation.pathParamNames)}`,
      ];

      if (summaryField.length > 0) {
        tokens.push(summaryField.replace(leadingSummaryFieldPrefixPattern, ""));
      }

      return `  { ${tokens.join(", ")} },`;
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
  readonly functionName: string;
  readonly domainName: string;
  readonly domainMethodName: string;
  readonly pathParamNames: readonly string[];
};

export const generatedOperations = [
${operationsLiteral}
] as const satisfies ReadonlyArray<GeneratedOperation>;

export type GeneratedOperationId = (typeof generatedOperations)[number]["id"];
export type GeneratedOperationTag = (typeof generatedOperations)[number]["tag"];
export type GeneratedOperationFunctionName = (typeof generatedOperations)[number]["functionName"];

export type GeneratedOperationById = {
  readonly [K in GeneratedOperationId]: Extract<
    (typeof generatedOperations)[number],
    { readonly id: K }
  >;
};

export type GeneratedOperationsByTag = {
  readonly [K in GeneratedOperationTag]: ReadonlyArray<
    Extract<(typeof generatedOperations)[number], { readonly tag: K }>
  >;
};

export const generatedOperationMap = Object.fromEntries(
  generatedOperations.map((operation) => [operation.id, operation])
) as { readonly [K in GeneratedOperationId]: GeneratedOperationById[K] };

export const generatedOperationsByTag = generatedOperations.reduce(
  (acc, operation) => {
    const current = acc[operation.tag] ?? [];
    acc[operation.tag] = [...current, operation];
    return acc;
  },
  {} as Record<string, ReadonlyArray<(typeof generatedOperations)[number]>>
) as GeneratedOperationsByTag;

export const generatedOperationsByFunctionName = Object.fromEntries(
  generatedOperations.map((operation) => [operation.functionName, operation.id])
) as Readonly<Record<GeneratedOperationFunctionName, GeneratedOperationId>>;
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

import type { GeneratedOperationId } from "../../contracts/_generated/contracts";

export type GeneratedOperationSchema = {
  readonly request?: Schema.Schema.AnyNoContext;
  readonly response?: Schema.Schema.AnyNoContext;
};

const componentSchemas: Record<string, Schema.Schema.AnyNoContext> = {};
${componentAssignments}

export const generatedOperationSchemas = {
${operationSchemaEntries}
} as const satisfies {
  readonly [K in GeneratedOperationId]: GeneratedOperationSchema;
};

type InferRequestBody<TOperationSchema extends GeneratedOperationSchema> =
  TOperationSchema extends { readonly request: infer TRequestSchema }
    ? TRequestSchema extends Schema.Schema.AnyNoContext
      ? Schema.Schema.Type<TRequestSchema>
      : undefined
    : undefined;

type InferResponseBody<TOperationSchema extends GeneratedOperationSchema> =
  TOperationSchema extends { readonly response: infer TResponseSchema }
    ? TResponseSchema extends Schema.Schema.AnyNoContext
      ? Schema.Schema.Type<TResponseSchema>
      : unknown
    : unknown;

export type OperationRequestBodyById = {
  readonly [K in GeneratedOperationId]: InferRequestBody<
    (typeof generatedOperationSchemas)[K]
  >;
};

export type OperationResponseBodyById = {
  readonly [K in GeneratedOperationId]: InferResponseBody<
    (typeof generatedOperationSchemas)[K]
  >;
};

export type OperationRequestBody<TOperationId extends GeneratedOperationId> =
  OperationRequestBodyById[TOperationId];

export type OperationResponseBody<TOperationId extends GeneratedOperationId> =
  OperationResponseBodyById[TOperationId];
`;
};

const renderOperationTypesFile = (
  operations: readonly ParsedOperation[]
): string => {
  const pathMap = operations
    .map(
      (operation) =>
        `  ${JSON.stringify(operation.id)}: ${operation.pathParamsTypeExpr};`
    )
    .join("\n");

  const queryMap = operations
    .map(
      (operation) =>
        `  ${JSON.stringify(operation.id)}: ${operation.queryTypeExpr};`
    )
    .join("\n");

  const headerMap = operations
    .map(
      (operation) =>
        `  ${JSON.stringify(operation.id)}: ${operation.headersTypeExpr};`
    )
    .join("\n");

  const metadataMap = operations
    .map((operation) => {
      const summaryField = operation.summary
        ? `, summary: ${JSON.stringify(operation.summary)}`
        : "";
      return `  ${JSON.stringify(operation.id)}: { functionName: ${JSON.stringify(operation.functionName)}, domainName: ${JSON.stringify(operation.domainName)}, domainMethodName: ${JSON.stringify(operation.domainMethodName)}, pathParamNames: ${JSON.stringify(operation.pathParamNames)}${summaryField} },`;
    })
    .join("\n");

  return `/* eslint-disable */
// This file is generated by scripts/generate-contracts.ts

import type { GeneratedOperationId } from "../../contracts/_generated/contracts";
import type {
  OperationRequestBodyById,
  OperationResponseBodyById,
} from "../../schema/_generated/schema";

export type OperationPathParamsById = {
${pathMap}
};

export type OperationQueryParamsById = {
${queryMap}
};

export type OperationHeaderParamsById = {
${headerMap}
};

export type OperationPathParams<TOperationId extends GeneratedOperationId> =
  OperationPathParamsById[TOperationId];

export type OperationQueryParams<TOperationId extends GeneratedOperationId> =
  OperationQueryParamsById[TOperationId];

export type OperationHeaderParams<TOperationId extends GeneratedOperationId> =
  OperationHeaderParamsById[TOperationId];

export type OperationInputById = {
  readonly [K in GeneratedOperationId]: {
    readonly pathParams: OperationPathParamsById[K];
    readonly query: OperationQueryParamsById[K];
    readonly headers: OperationHeaderParamsById[K];
    readonly body: OperationRequestBodyById[K];
    readonly response: OperationResponseBodyById[K];
  };
};

export type OperationInput<TOperationId extends GeneratedOperationId> =
  OperationInputById[TOperationId];

export const generatedOperationMetadataById = {
${metadataMap}
} as const satisfies {
  readonly [K in GeneratedOperationId]: {
    readonly functionName: string;
    readonly domainName: string;
    readonly domainMethodName: string;
    readonly pathParamNames: readonly string[];
    readonly summary?: string;
  };
};

export type GeneratedOperationMetadataById = typeof generatedOperationMetadataById;
`;
};

const scriptDir = fileURLToPath(new URL(".", import.meta.url));
const packageRoot = resolve(scriptDir, "..");
const workspaceRoot = resolve(packageRoot, "..", "..");

const openApiPath = resolve(workspaceRoot, "docs/openapi.yaml");
const contractsDir = resolve(packageRoot, "src/internal/contracts");
const schemaDir = resolve(packageRoot, "src/internal/schema");
const typesDir = resolve(packageRoot, "src/internal/types");

mkdirSync(contractsDir, { recursive: true });
mkdirSync(schemaDir, { recursive: true });
mkdirSync(typesDir, { recursive: true });
mkdirSync(resolve(contractsDir, "_generated"), { recursive: true });
mkdirSync(resolve(schemaDir, "_generated"), { recursive: true });
mkdirSync(resolve(typesDir, "_generated"), { recursive: true });

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
writeFileSync(
  resolve(typesDir, "_generated/operations.ts"),
  renderOperationTypesFile(operations),
  "utf-8"
);

console.log(`Generated ${operations.length} operations plus typed metadata`);
