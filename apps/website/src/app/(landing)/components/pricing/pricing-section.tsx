import Shell from "@/components/layout/shell";
import { StaticPricingTable } from "./static-pricing-table";

export function PricingSection() {
  return (
    <section className="py-20 sm:py-28">
      <Shell variant="default">
        <div className="mx-auto w-full max-w-2xl">
          <div className="flex flex-col gap-4">
            <span className="font-semibold text-[0.7rem] text-primary/80 uppercase tracking-[0.35em] sm:text-xs">
              PRICING
            </span>

            <div className="space-y-3">
              <h2 className="text-balance font-semibold text-3xl text-foreground tracking-tight sm:text-4xl lg:text-5xl">
                Choose the plan that fits your needs
              </h2>
              <p className="text-muted-foreground text-sm leading-relaxed sm:text-base">
                All plans include AI-powered writing assistance, style analysis,
                document management, and multi-format export. Save 20% with
                yearly billing.
              </p>
            </div>

            <StaticPricingTable />
          </div>
        </div>
      </Shell>
    </section>
  );
}
