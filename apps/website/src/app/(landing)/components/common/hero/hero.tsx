import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const Hero = () => (
  <section className="relative isolate overflow-hidden pt-32 pb-12 sm:pt-40 sm:pb-16">
    <div className="parallax-slow absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
    <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.14]" />
    <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
    <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

    <Shell variant="wide">
      <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
        <span className="kicker animate-fade-in-up">
          Job orchestration your team can ship with
        </span>

        <h1 className="mt-6 animate-delay-100 animate-fade-in-up text-balance text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl xl:text-7xl">
          <span className="font-bold text-foreground">
            Run every background workflow from one clean control center.
          </span>{" "}
          <span className="text-muted-foreground">
            Launch faster, fix failures sooner, and stop stitching together
            queue tools.
          </span>
        </h1>

        <p className="mt-5 max-w-3xl animate-delay-200 animate-fade-in-up text-base text-muted-foreground/70 leading-relaxed sm:mt-6 sm:text-lg">
          Strait gives your team one place to trigger work, watch progress, and
          recover quickly when something goes wrong.
        </p>

        <div className="mt-10 flex animate-delay-300 animate-fade-in-up flex-col items-center gap-4 sm:flex-row">
          <Button
            className="gradient-warm text-white shadow-sm transition-shadow duration-300 hover:shadow-md"
            render={<Link href={dashboardHref("/login")} />}
            size="lg"
          >
            Start your first workflow
            <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
          </Button>
          <a
            className="text-muted-foreground text-sm transition-colors hover:text-foreground"
            href="/docs/quickstart"
          >
            Read quickstart →
          </a>
        </div>

        <div className="mt-6 flex animate-delay-400 animate-fade-in-up flex-wrap items-center justify-center gap-2.5">
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            No broker setup required
          </span>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            Works with your PostgreSQL stack
          </span>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            Built for real production traffic
          </span>
        </div>
      </div>
    </Shell>
  </section>
);

export default Hero;
