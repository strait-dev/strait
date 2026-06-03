import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const sourceRoot = join(process.cwd(), "src");

describe("checkout policy", () => {
  it("does not enable Stripe promotion codes for launch checkout", () => {
    const checkoutFiles = [
      "routes/app/upgrade.tsx",
      "components/billing/addons-tab.tsx",
    ];

    for (const file of checkoutFiles) {
      const source = readFileSync(join(sourceRoot, file), "utf8");
      expect(source).not.toContain("allow_promotion_codes: true");
    }
  });
});
