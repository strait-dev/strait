import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import { StaticPricingTable } from "@/components/landing-page/pricing/static-pricing-table.tsx";
import Shell from "@/components/layout/shell.tsx";

const PricingTeaser = () => (
  <section className="py-20 sm:py-28">
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            Free to start. Pay as you grow.
          </h2>
          <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
            Every plan includes all core features. Upgrade when you need more
            runs, more compute, or team controls.
          </p>
        </div>
      </Reveal>

      <StaggerGroup className="mb-8 flex flex-wrap items-center gap-2.5">
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            No credit card required
          </span>
        </StaggerItem>
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            14-day trial on paid plans
          </span>
        </StaggerItem>
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            Cancel anytime
          </span>
        </StaggerItem>
      </StaggerGroup>

      <StaticPricingTable />

      <Reveal delay={0.3}>
        <div className="mt-10 flex flex-col items-center gap-3 text-center">
          <p className="max-w-lg text-muted-foreground text-sm">
            All paid plans include compute credits. Additional runs at $0.20 per
            1,000.
          </p>
          <a
            className="font-medium text-muted-foreground text-sm transition-colors hover:text-foreground"
            href="/pricing"
          >
            {"Compare all plans \u2192"}
          </a>
        </div>
      </Reveal>
    </Shell>
  </section>
);

export default PricingTeaser;
