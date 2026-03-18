import fs from "node:fs";
import path from "node:path";
import YAML from "yaml";
import { createOpenAPI } from "fumadocs-openapi/server";

export const openapi = createOpenAPI({
  input: () => {
    const specPath = path.resolve(process.cwd(), "../../docs/openapi.yaml");
    const specContent = fs.readFileSync(specPath, "utf-8");
    const specDocument = YAML.parse(specContent, {});
    return { openapi: specDocument };
  },
});
