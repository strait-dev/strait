"use client";

import { HugeiconsIcon, type HugeiconsIconProps } from "@hugeicons/react";
import { cn } from "@strait/ui/utils";
import Link from "next/link";
import { type ReactNode, useCallback, useRef, useState } from "react";

import Shell from "@/components/layout/shell";

type SupportingFeature = {
  title: string;
  description: string;
  icon: HugeiconsIconProps["icon"];
};

type FeatureShowcaseProps = {
  title: string;
  description: string;
  cta?: {
    href: string;
    label: string;
  };
  features: SupportingFeature[];
  visuals?: ReactNode[];
  orientation?: "visual-right" | "visual-left";
  id?: string;
  className?: string;
};

const FeatureShowcase = ({
  title,
  description,
  cta,
  features,
  visuals = [],
  orientation = "visual-right",
  id,
  className,
}: FeatureShowcaseProps) => {
  const [activeIndex, setActiveIndex] = useState(0);
  const visualRef = useRef<HTMLDivElement>(null);
  const hasVisuals = visuals.length > 0;
  const isReversed = orientation === "visual-left";

  const handleTabClick = useCallback(
    (index: number) => {
      setActiveIndex(index);
      if (hasVisuals && visualRef.current && window.innerWidth < 1024) {
        visualRef.current.scrollIntoView({
          behavior: "smooth",
          block: "nearest",
        });
      }
    },
    [hasVisuals]
  );

  return (
    <section className={cn("py-20 sm:py-28", className)} id={id}>
      <Shell variant="wide">
        <div
          className={cn(
            "grid gap-10 lg:gap-12",
            hasVisuals
              ? "lg:grid-cols-[1fr_1.2fr] lg:items-stretch"
              : "lg:grid-cols-1",
            isReversed && hasVisuals && "lg:[&>div:first-child]:order-last"
          )}
        >
          {/* Left: section info + feature tabs */}
          <div className="order-2 flex flex-col gap-6 lg:order-none">
            <div className="space-y-4">
              <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
                <span className="font-bold text-foreground">{title}.</span>{" "}
                <span className="text-muted-foreground">{description}</span>
              </h2>
            </div>

            <div
              aria-label="Feature highlights"
              className="flex flex-col gap-1"
              role="tablist"
            >
              {features.map((feature, index) => {
                const tabId = `feature-tab-${String(index)}`;
                const panelId = `feature-panel-${String(index)}`;

                return (
                  <button
                    aria-controls={panelId}
                    aria-selected={activeIndex === index}
                    className={cn(
                      "group relative flex min-h-11 items-start gap-3 overflow-hidden rounded-lg border border-transparent px-4 py-3 text-left transition-all duration-200 before:absolute before:top-2 before:bottom-2 before:left-0 before:w-0.5 before:rounded-full before:bg-primary before:transition-opacity focus-visible:ring-2 focus-visible:ring-primary/40",
                      activeIndex === index
                        ? "border-primary/30 bg-primary/5 before:opacity-100"
                        : "before:opacity-0 hover:bg-muted/50"
                    )}
                    id={tabId}
                    key={feature.title}
                    onClick={() => handleTabClick(index)}
                    role="tab"
                    type="button"
                  >
                    <span
                      className={cn(
                        "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors duration-200",
                        activeIndex === index
                          ? "bg-primary/10 text-primary"
                          : "bg-muted text-muted-foreground group-hover:text-foreground"
                      )}
                    >
                      <HugeiconsIcon className="size-4" icon={feature.icon} />
                    </span>
                    <div className="min-w-0 flex-1">
                      <h3
                        className={cn(
                          "font-medium text-sm transition-colors duration-200",
                          activeIndex === index
                            ? "text-foreground"
                            : "text-muted-foreground group-hover:text-foreground"
                        )}
                      >
                        {feature.title}
                      </h3>
                      <p
                        className={cn(
                          "mt-0.5 text-sm leading-relaxed transition-colors duration-200",
                          activeIndex === index
                            ? "text-muted-foreground"
                            : "text-muted-foreground/60"
                        )}
                      >
                        {feature.description}
                      </p>
                    </div>
                  </button>
                );
              })}
            </div>

            {!!cta && (
              <div className="mt-2">
                <Link
                  className="inline-flex items-center gap-2 font-semibold text-primary text-sm transition-colors hover:text-primary/80"
                  href={cta.href}
                >
                  {cta.label}
                  <span aria-hidden>→</span>
                </Link>
              </div>
            )}
          </div>

          {hasVisuals && (
            <div
              className="order-1 flex flex-col lg:sticky lg:top-32 lg:order-none"
              ref={visualRef}
            >
              <div className="flex flex-1 flex-col rounded-2xl bg-primary/20 p-3 sm:p-4">
                <div className="paper-texture relative flex-1 overflow-hidden rounded-xl border border-primary/15 bg-card shadow-lg">
                  {visuals.map((visual, index) => (
                    <div
                      aria-hidden={activeIndex !== index}
                      aria-labelledby={`feature-tab-${String(index)}`}
                      className={cn(
                        "transition-all duration-300",
                        activeIndex === index
                          ? "relative opacity-100"
                          : "pointer-events-none absolute inset-0 opacity-0"
                      )}
                      id={`feature-panel-${String(index)}`}
                      key={features[index]?.title ?? `visual-${String(index)}`}
                      role="tabpanel"
                    >
                      {visual}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </div>
      </Shell>
    </section>
  );
};

export default FeatureShowcase;
