import { formatPlanPrice, PLANS } from "@strait/billing/products";
import { Suspense } from "react";

import CTA from "@/app/(landing)/components/common/cta/cta.tsx";
import { StaticPricingTable } from "@/app/(landing)/components/pricing/static-pricing-table.tsx";
import Shell from "@/components/layout/shell.tsx";
import PricingComparison from "@/components/pricing/pricing-comparison.tsx";
import PricingFaq, {
  PRICING_FAQ_ITEMS,
} from "@/components/pricing/pricing-faq.tsx";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import {
  getFAQPageSchema,
  getPricingProductsSchema,
  getSoftwareApplicationSchema,
  JsonLd,
  JsonLdMultiple,
} from "@/lib/structured-data.tsx";

export const metadata = generatePageMetadata({
  title: "Pricing",
  description:
    "Four plans from free to enterprise. All core features included. Pay only for scale.",
  path: "/pricing",
  keywords: [
    "Strait pricing",
    "job orchestration pricing",
    "workflow platform plans",
    "background job platform subscription",
    "Strait plans",
  ],
});

export default function PricingPage() {
  const starterMonthly = formatPlanPrice(PLANS.starter, "yearly");
  const proMonthly = formatPlanPrice(PLANS.pro, "yearly");

  const softwareAppSchema = getSoftwareApplicationSchema();
  const pricingProductsSchema = getPricingProductsSchema();
  const faqSchema = getFAQPageSchema(PRICING_FAQ_ITEMS);

  return (
    <main className="pt-32 sm:pt-40">
      <JsonLd data={softwareAppSchema} />
      <JsonLdMultiple data={pricingProductsSchema} />
      {faqSchema ? <JsonLd data={faqSchema} /> : null}

      <section className="relative isolate overflow-hidden pb-16 sm:pb-20">
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
        <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

        <Shell variant="wide">
          <div className="mx-auto max-w-3xl text-center">
            <span className="kicker">Pricing</span>
            <h1 className="mt-6 text-balance text-4xl leading-[1.12] tracking-tight sm:text-5xl lg:text-6xl">
              <span className="text-foreground">
                Simple pricing, built for reliable orchestration.
              </span>{" "}
              <span className="text-muted-foreground">
                Start free with all core features. Scale when you are ready.
              </span>
            </h1>
            <p className="mt-6 text-pretty text-base text-muted-foreground/70 leading-relaxed sm:text-lg">
              No hidden fees. Cancel anytime. Self-host or let us run it for
              you.
            </p>

            <div className="mt-8 flex flex-wrap items-center justify-center gap-2.5">
              <span className="rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm">
                Free $0
              </span>
              <span className="rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm">
                Starter from {starterMonthly}/mo
              </span>
              <span className="rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm">
                Pro from {proMonthly}/mo
              </span>
              <span className="rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm">
                Enterprise Custom
              </span>
              <span className="rounded-full border border-foreground/10 bg-muted/60 px-4 py-1.5 font-medium text-foreground text-xs sm:text-sm">
                Save ~17% annually
              </span>
            </div>
          </div>
        </Shell>
      </section>

      <section className="pb-20 sm:pb-28">
        <Shell variant="wide">
          <div className="mx-auto max-w-3xl">
            <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
              <span className="text-foreground">
                Every plan includes all core features.
              </span>{" "}
              <span className="text-muted-foreground">
                Pay only for scale and governance.
              </span>
            </h2>
          </div>

          <StaticPricingTable />
        </Shell>
      </section>

      <PricingComparison />

      <Suspense fallback={null}>
        <PricingFaq />
      </Suspense>

      <CTA />
    </main>
  );
}
