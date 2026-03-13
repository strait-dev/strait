import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";

import { dashboardHref } from "@/lib/urls.ts";

const CTA = () => {
  const title = "Ready to run every workflow from one place?";
  const description =
    "Give your team a cleaner way to launch work, monitor progress, and recover fast when runs fail.";
  const buttonText = "Start with Strait";
  const buttonHref = "/login";
  const subtext =
    "Built for modern engineering teams that want fewer moving parts and faster recovery.";

  const headingId = "cta-title";

  return (
    <section
      aria-labelledby={headingId}
      className="relative border-border/40 border-y bg-primary py-20 sm:py-28"
    >
      <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.12]" />
      <div className="showcase-dots pointer-events-none absolute inset-0 opacity-15" />

      <Shell className="relative z-10" variant="wide">
        <div className="flex flex-col items-center text-center">
          <h2
            className="max-w-3xl text-2xl text-primary-foreground leading-[1.1] tracking-tighter sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            {title}
          </h2>

          <p className="mt-6 max-w-2xl text-base text-primary-foreground/70 leading-relaxed sm:text-lg">
            {description}
          </p>

          <div className="mt-10 flex flex-col items-center gap-4">
            <Button
              render={<Link href={dashboardHref(buttonHref)} />}
              variant="outline"
            >
              {buttonText}
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
            <p className="text-primary-foreground/50 text-sm">{subtext}</p>
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
