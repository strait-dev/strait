import { Schema } from "effect";

const trailingSlashPattern = /\/+$/;

export const AuthModeSchema = Schema.Union(
  Schema.Struct({
    type: Schema.Literal("bearer"),
    token: Schema.String,
  }),
  Schema.Struct({
    type: Schema.Literal("apiKey"),
    token: Schema.String,
  }),
  Schema.Struct({
    type: Schema.Literal("runToken"),
    token: Schema.String,
  })
);

export const StraitClientConfigSchema = Schema.Struct({
  baseUrl: Schema.String,
  auth: AuthModeSchema,
  defaultHeaders: Schema.optional(
    Schema.Record({ key: Schema.String, value: Schema.String })
  ),
  timeoutMs: Schema.optional(
    Schema.Number.pipe(Schema.int(), Schema.positive())
  ),
});

export type AuthMode = Schema.Schema.Type<typeof AuthModeSchema>;
export type StraitClientConfigInput = Schema.Schema.Encoded<
  typeof StraitClientConfigSchema
>;
export type StraitClientConfig = Schema.Schema.Type<
  typeof StraitClientConfigSchema
>;

export const normalizeBaseUrl = (baseUrl: string): string =>
  baseUrl.replace(trailingSlashPattern, "");

export const getAuthorizationHeader = (auth: AuthMode): string => {
  switch (auth.type) {
    case "apiKey":
      return `Bearer ${auth.token}`;
    case "bearer":
      return `Bearer ${auth.token}`;
    case "runToken":
      return `Bearer ${auth.token}`;
    default:
      return "";
  }
};
