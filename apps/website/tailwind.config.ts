import baseConfig from "@strait/ui/tailwind.config";
import type { Config } from "tailwindcss";

export default {
  content: ["./src/**/*.{ts,tsx}", "../../packages/ui/src/**/*.{ts,tsx}"],
  presets: [baseConfig],
  darkMode: ["class", "[data-mode='dark']"],
} satisfies Config;
