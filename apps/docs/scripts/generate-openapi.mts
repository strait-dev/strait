import fs from "node:fs";
import path from "node:path";
import YAML from "yaml";
import { generateFiles } from "fumadocs-openapi";
import { createOpenAPI } from "fumadocs-openapi/server";

const specPath = path.resolve(
  import.meta.dirname,
  "../../../docs/openapi.yaml"
);
const specContent = fs.readFileSync(specPath, "utf-8");
const specDocument = YAML.parse(specContent, {});

const openapi = createOpenAPI({
  input: () => ({ openapi: specDocument }),
});

generateFiles({
  input: openapi,
  output: "./content/docs/api-reference",
  per: "tag",
}).catch(console.error);
