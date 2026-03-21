import Shell from "@/components/layout/shell.tsx";
import { StaticPricingTable } from "./static-pricing-table.tsx";

export function PricingSection() {
  return (
    <section className="py-20 sm:py-28">
      <Shell variant="default">
        <div className="mx-auto w-full max-w-2xl">
          <div className="flex flex-col gap-4">
            <span className="kicker">PRICING</span>

            <div className="space-y-3">
              <h2 className="text-balance font-semibold text-2xl text-foreground sm:text-3xl lg:text-4xl">
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
