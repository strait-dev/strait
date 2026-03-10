import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import { dashboardHref } from "@/lib/urls";

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
      className="relative border-border/40 border-y bg-primary/10 py-20 sm:py-28"
    >
      <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.12]" />
      <div className="showcase-dots pointer-events-none absolute inset-0 opacity-15" />
      <div
        className="pointer-events-none absolute inset-0 opacity-10"
        style={{
          background:
            "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.08), transparent 60%)",
        }}
      />

      <div className="relative z-10 mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="flex flex-col items-center text-center">
          <h2
            className="max-w-3xl font-bold text-3xl text-foreground leading-[1.1] tracking-tighter sm:text-4xl lg:text-5xl"
            id={headingId}
          >
            {title}
          </h2>

          <p className="mt-6 max-w-2xl text-base text-muted-foreground leading-relaxed sm:text-lg">
            {description}
          </p>

          <div className="mt-10 flex flex-col items-center gap-4">
            <Button
              className="border border-primary/30 bg-primary/12 text-foreground transition-colors duration-300 hover:bg-primary/18"
              render={<Link href={dashboardHref(buttonHref)} />}
              size="lg"
              variant="outline"
            >
              {buttonText}
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
            <p className="text-muted-foreground text-sm">{subtext}</p>
          </div>
        </div>
      </div>
    </section>
  );
};

export default CTA;
