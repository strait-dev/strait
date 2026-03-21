import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";

type PageHeroProps = {
  title?: string;
  highlightedTitle?: string;
  description?: string;
  primaryCTA?: {
    text: string;
    href: string;
  };
  secondaryCTA?: {
    text: string;
    href: string;
  };
};

/**
 * Simplified hero component for internal pages (blog, features, etc.)
 * The main Hero component is used only on the homepage with richer CMS data.
 */
const PageHero = ({
  title = "Strait",
  highlightedTitle,
  description = "Built for modern commerce teams.",
  primaryCTA = {
    text: "Get started",
    href: "/get-started",
  },
  secondaryCTA,
}: PageHeroProps) => {
  return (
    <section className="relative isolate overflow-hidden py-20 sm:py-28">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_top,_hsl(var(--primary)/0.2),_transparent_55%)]" />
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_hsl(var(--background))_70%)]" />

      <Shell variant="wide">
        <div className="mx-auto flex max-w-3xl flex-col items-center text-center">
          <h1 className="mt-6 text-balance text-4xl text-foreground sm:text-5xl lg:text-6xl">
            {title}{" "}
            {highlightedTitle ? (
              <span className="mt-2 block text-foreground sm:mt-0 sm:inline sm:whitespace-nowrap">
                {highlightedTitle}
              </span>
            ) : null}
          </h1>

          <p className="mt-5 max-w-2xl text-balance text-lg text-muted-foreground leading-relaxed sm:mt-6 sm:text-xl">
            {description}
          </p>

          <div className="mt-10 flex flex-col items-center gap-3">
            <div className="flex flex-col items-center gap-3 sm:flex-row sm:gap-4">
              <Button
                className="w-full shadow-sm transition-shadow duration-300 hover:shadow-md sm:w-auto"
                render={<Link href={primaryCTA.href} />}
              >
                {primaryCTA.text}
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              {secondaryCTA ? (
                <Button
                  className="w-full backdrop-blur-sm sm:w-auto"
                  render={<Link href={secondaryCTA.href} />}
                  variant="outline"
                >
                  {secondaryCTA.text}
                  <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default PageHero;
