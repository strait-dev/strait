import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { notFound } from "next/navigation";

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

function FeatureValue({ value }: { value: string | boolean }) {
  if (typeof value === "boolean") {
    if (value) {
      return (
        <span
          aria-label="Yes"
          className="font-medium text-emerald-500"
          role="img"
        >
          &#10003;
        </span>
      );
    }
    return (
      <span aria-label="No" className="text-muted-foreground/50" role="img">
        &#10005;
      </span>
    );
  }
  return <span className="text-muted-foreground text-sm">{value}</span>;
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
        <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.08]" />
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
            <h1 className="mt-4 text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
              <span className="text-foreground">
                Strait vs {comparison.competitor}.
              </span>{" "}
              <span className="text-muted-foreground">
                {comparison.tagline}
              </span>
            </h1>
            <p className="mt-6 max-w-2xl text-lg text-muted-foreground/70 leading-relaxed">
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
          <h2 className="mb-10 text-2xl tracking-tight sm:text-3xl">
            Key differences
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {comparison.differentiators.map((diff) => (
              <div
                className="rounded-xl border border-border/60 bg-card p-6"
                key={diff.title}
              >
                <h3 className="font-semibold text-foreground">{diff.title}</h3>
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
            ))}
          </div>
        </Shell>
      </section>

      {/* Comparison Table */}
      <section className="py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl tracking-tight sm:text-3xl">
            Feature comparison
          </h2>
          <div className="space-y-6">
            {comparison.categories.map((category) => (
              <details
                className="group rounded-xl border border-border/60 bg-card"
                key={category.name}
                open
              >
                <summary className="cursor-pointer select-none px-6 py-4 font-semibold text-foreground">
                  {category.name}
                </summary>
                <div className="border-border/40 border-t">
                  {/* Header row */}
                  <div className="grid grid-cols-3 gap-4 border-border/30 border-b px-6 py-3">
                    <p className="text-muted-foreground text-sm">Feature</p>
                    <p className="text-center text-muted-foreground text-sm">
                      Strait
                    </p>
                    <p className="text-center text-muted-foreground text-sm">
                      {comparison.competitor}
                    </p>
                  </div>
                  {/* Feature rows */}
                  {category.features.map((row) => (
                    <div
                      className="grid grid-cols-3 gap-4 border-border/20 border-b px-6 py-3 last:border-b-0"
                      key={row.feature}
                    >
                      <p className="text-foreground text-sm">{row.feature}</p>
                      <p className="text-center">
                        <FeatureValue value={row.strait} />
                      </p>
                      <p className="text-center">
                        <FeatureValue value={row.competitor} />
                      </p>
                    </div>
                  ))}
                </div>
              </details>
            ))}
          </div>
        </Shell>
      </section>

      {/* Switching Steps */}
      <section className="border-border/40 border-t py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl tracking-tight sm:text-3xl">
            Switching from {comparison.competitor}
          </h2>
          <ol className="space-y-4">
            {comparison.switchingSteps.map((step, index) => {
              const stepNumber = index + 1;
              return (
                <li className="flex gap-4" key={step}>
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary font-medium text-primary-foreground text-sm">
                    {stepNumber}
                  </span>
                  <p className="pt-1 text-muted-foreground leading-relaxed">
                    {step}
                  </p>
                </li>
              );
            })}
          </ol>
        </Shell>
      </section>

      {/* CTA */}
      <section className="border-border/40 border-t bg-primary py-16 sm:py-20">
        <Shell className="text-center" variant="wide">
          <h2 className="text-2xl text-primary-foreground tracking-tight sm:text-3xl">
            Ready to switch from {comparison.competitor}?
          </h2>
          <p className="mt-4 text-primary-foreground/70">
            Deploy your first workflow in under 10 minutes.
          </p>
          <div className="mt-8">
            <Button
              render={<Link href={dashboardHref("/login")} />}
              variant="outline"
            >
              Get started
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
          </div>
        </Shell>
      </section>
    </>
  );
}
