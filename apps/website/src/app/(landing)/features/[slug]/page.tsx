import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { notFound } from "next/navigation";

import MeshGradientBg from "@/components/landing/mesh-gradient-bg.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import Particles from "@/components/magicui/particles.tsx";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import { getBreadcrumbSchema, JsonLd } from "@/lib/structured-data.tsx";
import { dashboardHref } from "@/lib/urls.ts";
import {
  FEATURE_PAGES,
  getAllFeatureSlugs,
  getFeatureBySlug,
} from "../data.ts";

type Props = {
  params: Promise<{ slug: string }>;
};

export function generateStaticParams() {
  return getAllFeatureSlugs().map((slug) => ({ slug }));
}

export async function generateMetadata({ params }: Props) {
  const { slug } = await params;
  const feature = getFeatureBySlug(slug);
  if (!feature) {
    return {};
  }
  return generatePageMetadata({
    title: `${feature.name} — Strait Features`,
    description: feature.description,
    path: `/features/${feature.slug}`,
    keywords: [feature.name, "Strait", "job orchestration", "workflow"],
  });
}

export default async function FeaturePage({ params }: Props) {
  const { slug } = await params;
  const feature = getFeatureBySlug(slug);
  if (!feature) {
    notFound();
  }

  const BASE_URL =
    process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
  const breadcrumbs = getBreadcrumbSchema([
    { name: "Home", url: BASE_URL },
    { name: "Features", url: `${BASE_URL}/features` },
    { name: feature.name, url: `${BASE_URL}/features/${feature.slug}` },
  ]);

  const related = feature.relatedSlugs
    .map((s) => FEATURE_PAGES.find((f) => f.slug === s))
    .filter(Boolean);

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
              href="/features"
            >
              Features
            </Link>
            <span>/</span>
            <span className="text-foreground">{feature.name}</span>
          </nav>

          <div className="max-w-3xl">
            <span className="kicker">{feature.name}</span>
            <Reveal variant="blur">
              <h1 className="mt-4 text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
                <span className="text-foreground">{feature.headline}</span>{" "}
                <span className="text-muted-foreground">
                  {feature.subheadline}
                </span>
              </h1>
            </Reveal>
            <p className="mt-6 max-w-2xl text-lg text-muted-foreground/70 leading-relaxed">
              {feature.description}
            </p>
            <div className="mt-8">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Try it now
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </div>
        </Shell>
      </section>

      {/* Specs Grid */}
      <section className="border-border/40 border-y py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl tracking-tight sm:text-3xl">
            Technical specs
          </h2>
          <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {feature.specs.map((spec) => (
              <StaggerItem key={spec.label}>
                <div className="rounded-xl border border-border/60 bg-card p-5">
                  <p className="text-muted-foreground text-sm">{spec.label}</p>
                  <p className="mt-1 font-medium text-foreground">
                    {spec.value}
                  </p>
                </div>
              </StaggerItem>
            ))}
          </StaggerGroup>
        </Shell>
      </section>

      {/* Code Example */}
      <section className="py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-2 text-2xl tracking-tight sm:text-3xl">
            {feature.codeExample.title}
          </h2>
          <Reveal variant="scale">
            <MockBrowserWindow
              className="mt-8"
              url={`main.${feature.codeExample.language}`}
            >
              <pre className="overflow-x-auto p-6 font-mono text-sm leading-relaxed">
                <code className="text-foreground/80">
                  {feature.codeExample.code}
                </code>
              </pre>
            </MockBrowserWindow>
          </Reveal>
        </Shell>
      </section>

      {/* Related Features */}
      {related.length > 0 && (
        <section className="border-border/40 border-t py-16 sm:py-20">
          <Shell variant="wide">
            <h2 className="mb-8 text-2xl tracking-tight sm:text-3xl">
              Related features
            </h2>
            <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {related.map((r) => {
                if (!r) {
                  return null;
                }
                return (
                  <StaggerItem key={r.slug}>
                    <Link
                      className="group block rounded-xl border border-border/60 bg-card p-6 transition-shadow hover:shadow-md"
                      href={`/features/${r.slug}`}
                    >
                      <h3 className="font-semibold text-foreground group-hover:text-primary">
                        {r.name}
                      </h3>
                      <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                        {r.subheadline}
                      </p>
                    </Link>
                  </StaggerItem>
                );
              })}
            </StaggerGroup>
          </Shell>
        </section>
      )}

      {/* CTA */}
      <section className="relative border-border/40 border-t bg-primary py-16 sm:py-20">
        <MeshGradientBg />
        <Particles
          className="pointer-events-none absolute inset-0"
          color="var(--background)"
          quantity={80}
          size={0.4}
          staticity={40}
        />
        <Shell className="relative z-10 text-center" variant="wide">
          <h2 className="text-2xl text-primary-foreground tracking-tight sm:text-3xl">
            Ready to try {feature.name}?
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
