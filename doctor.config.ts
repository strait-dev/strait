import { defineConfig } from "react-doctor/api";

export default defineConfig({
  ignore: {
    files: [
      "**/.output/**",
      "**/.nitro/**",
      "**/.tanstack/**",
      "**/.turbo/**",
      "**/dist/**",
      "**/node_modules/**",
      "**/routeTree.gen.ts",
    ],
  },
  noScore: true,
});
