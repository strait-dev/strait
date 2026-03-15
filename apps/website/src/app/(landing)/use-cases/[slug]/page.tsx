import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { notFound } from "next/navigation";

import LoadingCarousel from "@/components/cultui/loading-carousel.tsx";
import MeshGradientBg from "@/components/landing/mesh-gradient-bg.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import Particles from "@/components/magicui/particles.tsx";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import { getBreadcrumbSchema, JsonLd } from "@/lib/structured-data.tsx";
import { dashboardHref } from "@/lib/urls.ts";
import { getAllUseCaseSlugs, getUseCaseBySlug } from "../data.ts";

type Props = {
  params: Promise<{ slug: string }>;
};

export function generateStaticParams() {
  return getAllUseCaseSlugs().map((slug) => ({ slug }));
}

export async function generateMetadata({ params }: Props) {
  const { slug } = await params;
  const useCase = getUseCaseBySlug(slug);
  if (!useCase) {
    return {};
  }
  return generatePageMetadata({
    title: `${useCase.title} — Strait Use Cases`,
    description: useCase.solution,
    path: `/use-cases/${useCase.slug}`,
    keywords: [useCase.title, "Strait", "job orchestration", "use case"],
  });
}

export default async function UseCasePage({ params }: Props) {
  const { slug } = await params;
  const useCase = getUseCaseBySlug(slug);
  if (!useCase) {
    notFound();
  }

  const BASE_URL =
    process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
  const breadcrumbs = getBreadcrumbSchema([
    { name: "Home", url: BASE_URL },
    { name: "Use Cases", url: `${BASE_URL}/use-cases` },
    { name: useCase.title, url: `${BASE_URL}/use-cases/${useCase.slug}` },
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
              href="/use-cases"
            >
              Use Cases
            </Link>
            <span>/</span>
            <span className="text-foreground">{useCase.title}</span>
          </nav>

          <div className="max-w-3xl">
            <span className="kicker">{useCase.title}</span>
            <Reveal variant="blur">
              <h1 className="mt-4 text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
                <span className="text-foreground">{useCase.headline}</span>
              </h1>
            </Reveal>

            <div className="mt-8 grid grid-cols-1 gap-6 sm:grid-cols-2">
              <Reveal delay={0.1} variant="fade-left">
                <div className="rounded-xl border border-border/60 bg-card p-6">
                  <p className="font-medium text-foreground text-sm uppercase tracking-wider">
                    The Problem
                  </p>
                  <p className="mt-3 text-muted-foreground leading-relaxed">
                    {useCase.problem}
                  </p>
                </div>
              </Reveal>
              <Reveal delay={0.15} variant="fade-right">
                <div className="rounded-xl border border-primary/30 bg-primary/5 p-6">
                  <p className="font-medium text-primary text-sm uppercase tracking-wider">
                    The Solution
                  </p>
                  <p className="mt-3 text-muted-foreground leading-relaxed">
                    {useCase.solution}
                  </p>
                </div>
              </Reveal>
            </div>

            <div className="mt-8">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Get started
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </div>
        </Shell>
      </section>

      {/* Workflow Diagram */}
      <section className="border-border/40 border-y py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-10 text-2xl tracking-tight sm:text-3xl">
            How it works
          </h2>
          <LoadingCarousel
            autoplayDelay={4000}
            slides={useCase.workflowSteps.map((step, index) => ({
              number: index + 1,
              name: step.name,
              description: step.description,
            }))}
          />
        </Shell>
      </section>

      {/* Relevant Features */}
      <section className="py-16 sm:py-20">
        <Shell variant="wide">
          <h2 className="mb-8 text-2xl tracking-tight sm:text-3xl">
            Relevant features
          </h2>
          <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {useCase.relevantFeatures.map((feature) => (
              <StaggerItem key={feature.slug}>
                <Link
                  className="group block rounded-xl border border-border/60 bg-card p-6 transition-shadow hover:shadow-md"
                  href={`/features/${feature.slug}`}
                >
                  <h3 className="font-semibold text-foreground group-hover:text-primary">
                    {feature.name}
                  </h3>
                  <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                    {feature.description}
                  </p>
                </Link>
              </StaggerItem>
            ))}
          </StaggerGroup>
        </Shell>
      </section>

      {/* CTA */}
      <section className="relative border-border/40 border-t bg-primary py-16 sm:py-20">
        <MeshGradientBg />
        <Particles
          className="pointer-events-none absolute inset-0"
          color="var(--primary-foreground)"
          quantity={80}
          size={0.4}
          staticity={40}
        />
        <Shell className="relative z-10 text-center" variant="wide">
          <h2 className="text-2xl text-primary-foreground tracking-tight sm:text-3xl">
            Ready to build {useCase.title.toLowerCase()}?
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
