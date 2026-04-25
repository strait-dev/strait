import type { Config } from "tailwindcss";

export default {
  theme: {
    extend: {
      fontFamily: {
        sans: ["Geist", "system-ui", "sans-serif"],
        mono: ["Geist Mono", "monospace"],
      },
      keyframes: {
        "accordion-down": {
          from: { height: "0" },
          to: { height: "var(--radix-accordion-content-height)" },
        },
        "accordion-up": {
          from: { height: "var(--radix-accordion-content-height)" },
          to: { height: "0" },
        },
        jiggle: {
          "0%": { transform: "rotate(-4deg)" },
          "50%": { transform: "rotate(4deg)" },
        },
        "caret-blink": {
          "0%,70%,100%": { opacity: "1" },
          "20%,50%": { opacity: "0" },
        },
        "dot-pulse": {
          "0%, 100%": { opacity: "0.3", transform: "scale(0.85)" },
          "50%": { opacity: "1", transform: "scale(1)" },
        },
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
        "caret-blink": "caret-blink 1.25s ease-out infinite",
        "dot-pulse": "dot-pulse 1.4s cubic-bezier(0.4, 0, 0.2, 1) infinite",
      },
    },
  },
} satisfies Config;
