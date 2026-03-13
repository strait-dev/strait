import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const PricingHero = () => {
  const badge = "Pricing";
  const title = "Simple plans for serious writing";
  const subtitle =
    "Choose the plan that fits your workflow and scale when your output grows.";
  const ctaText = "Start writing";
  const ctaHref = "/login";
  const secondaryCtaText = "Compare plans";
  const secondaryCtaHref = "/pricing";

  return (
    <section className="relative isolate overflow-hidden pt-16 pb-12 sm:pt-20 sm:pb-16">
      <Shell variant="wide">
        <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
          <span className="kicker">{badge}</span>

          <h1 className="mt-4 text-balance text-4xl text-foreground tracking-tight sm:text-5xl lg:text-6xl">
            {title}
          </h1>

          <p className="mt-4 max-w-2xl text-balance text-lg text-muted-foreground leading-relaxed sm:mt-6 sm:text-xl">
            {subtitle}
          </p>

          <div className="mt-6 flex flex-col items-center gap-3 sm:mt-8 sm:flex-row sm:gap-4">
            <Button
              className="w-full sm:w-auto"
              render={<Link href={dashboardHref(ctaHref)} />}
            >
              {ctaText}
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
            <Button
              className="w-full sm:w-auto"
              render={<Link href={secondaryCtaHref} />}
              variant="outline"
            >
              {secondaryCtaText}
            </Button>
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default PricingHero;
