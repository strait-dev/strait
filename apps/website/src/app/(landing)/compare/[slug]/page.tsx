import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { notFound } from "next/navigation";
import CTA from "@/app/(landing)/components/common/cta/cta.tsx";
import ComparisonHighlights from "@/components/compare/comparison-highlights.tsx";
import ComparisonTable from "@/components/compare/comparison-table.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import { getBreadcrumbSchema, JsonLd } from "@/lib/structured-data.tsx";
import { dashboardHref } from "@/lib/urls.ts";
import { getAllComparisonSlugs, getComparisonBySlug } from "../data.ts";

type Props = {
  params: Promise<{ slug: string }>;
};

export function generateStaticParams() {
  return getAllComparisonSlugs().map((slug) => ({ slug }));
}

export async function generateMetadata({ params }: Props) {
  const { slug } = await params;
  const comparison = getComparisonBySlug(slug);
  if (!comparison) {
    return {};
  }
  return generatePageMetadata({
    title: `Strait vs ${comparison.competitor} — Compare`,
    description: comparison.description,
    path: `/compare/${comparison.slug}`,
    keywords: [
      "Strait",
      comparison.competitor,
      "comparison",
      "job orchestration",
      "workflow",
    ],
  });
}

export default async function ComparisonPage({ params }: Props) {
  const { slug } = await params;
  const comparison = getComparisonBySlug(slug);
  if (!comparison) {
    notFound();
  }

  const BASE_URL =
    process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
  const breadcrumbs = getBreadcrumbSchema([
    { name: "Home", url: BASE_URL },
    { name: "Compare", url: `${BASE_URL}/compare` },
    {
      name: `Strait vs ${comparison.competitor}`,
      url: `${BASE_URL}/compare/${comparison.slug}`,
    },
  ]);

  return (
    <>
      <JsonLd data={breadcrumbs} />

      {/* Hero */}
      <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-20">
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />

        <Shell variant="wide">
          {/* Breadcrumb */}
          <nav
            aria-label="Breadcrumb"
            className="mb-8 flex items-center gap-1.5 text-muted-foreground text-sm"
          >
            <Link className="transition-colors hover:text-foreground" href="/">
              Home
            </Link>
            <span>/</span>
            <Link
              className="transition-colors hover:text-foreground"
              href="/compare"
            >
              Compare
            </Link>
            <span>/</span>
            <span className="text-foreground">
              Strait vs {comparison.competitor}
            </span>
          </nav>

          <div className="max-w-3xl">
            <span className="kicker">Compare</span>
            <Reveal variant="blur">
              <h1 className="mt-4 text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
                <span className="text-foreground">
                  Strait vs {comparison.competitor}.
                </span>{" "}
                <span className="text-muted-foreground">
                  {comparison.tagline}
                </span>
              </h1>
            </Reveal>
            <p className="mt-6 max-w-2xl text-muted-foreground/70 text-sm leading-relaxed sm:text-base">
              {comparison.description}
            </p>
            <div className="mt-8">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Try Strait free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </div>
        </Shell>
      </section>

      {/* Differentiators */}
      <section className="border-border/40 border-y py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl sm:text-3xl">Key differences</h2>
          <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {comparison.differentiators.map((diff) => (
              <StaggerItem key={diff.title}>
                <div className="rounded-xl border border-border/60 bg-card p-6">
                  <h3 className="font-semibold text-foreground">
                    {diff.title}
                  </h3>
                  <div className="mt-4 space-y-3">
                    <div>
                      <p className="text-muted-foreground text-xs uppercase tracking-wide">
                        Strait
                      </p>
                      <p className="mt-1 text-foreground text-sm leading-relaxed">
                        {diff.strait}
                      </p>
                    </div>
                    <div>
                      <p className="text-muted-foreground text-xs uppercase tracking-wide">
                        {comparison.competitor}
                      </p>
                      <p className="mt-1 text-muted-foreground/70 text-sm leading-relaxed">
                        {diff.competitor}
                      </p>
                    </div>
                  </div>
                </div>
              </StaggerItem>
            ))}
          </StaggerGroup>
        </Shell>
      </section>

      {/* Feature Comparison Table */}
      <section className="py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl sm:text-3xl">Feature comparison</h2>
          <ComparisonTable
            categories={comparison.categories}
            competitorName={comparison.competitor}
          />
        </Shell>
      </section>

      {/* Feature Highlights */}
      {comparison.highlights.length > 0 && (
        <section className="border-border/40 border-t py-16 sm:py-20">
          <Shell variant="wide">
            <ComparisonHighlights
              competitorName={comparison.competitor}
              highlights={comparison.highlights}
            />
          </Shell>
        </section>
      )}

      {/* Switching Steps */}
      <section className="border-border/40 border-t py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl sm:text-3xl">
            Switching from {comparison.competitor}
          </h2>
          <StaggerGroup className="space-y-4">
            {comparison.switchingSteps.map((step, index) => {
              const stepNumber = index + 1;
              return (
                <StaggerItem key={step}>
                  <li className="flex gap-4">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary font-medium text-primary-foreground text-sm">
                      {stepNumber}
                    </span>
                    <p className="pt-1 text-muted-foreground leading-relaxed">
                      {step}
                    </p>
                  </li>
                </StaggerItem>
              );
            })}
          </StaggerGroup>
        </Shell>
      </section>

      <CTA
        description="Deploy your first workflow in under 10 minutes."
        heading={`Ready to switch from ${comparison.competitor}?`}
        showInstallSnippet={false}
      />
    </>
  );
}
