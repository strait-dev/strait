import { Suspense } from "react";

import { generateMetadata as generatePageMetadata } from "@/lib/metadata";
import {
  getHowToSchema,
  getOrganizationSchema,
  getSoftwareApplicationSchema,
  getWebSiteSchema,
  JsonLd,
} from "@/lib/structured-data";
import AudienceSection from "./components/audience/audience-section";
import WhyStrait from "./components/benefits/why-polyglot";
import CTA from "./components/common/cta/cta";
import InterviewShowcase from "./components/common/feature-section/interview-showcase";
import StylesShowcase from "./components/common/feature-section/styles-showcase";
import WritingToolkitShowcase from "./components/common/feature-section/writing-toolkit-showcase";
import Hero from "./components/common/hero/hero";
import InteractiveDemo from "./components/common/hero/interactive-demo";
import ProblemSection from "./components/common/hero/problem-section";
import ProductShowcase from "./components/common/hero/product-showcase";

import ComparisonSection from "./components/comparison/comparison-section";
import HowItWorks from "./components/how-it-works/how-it-works";
import PricingTeaser from "./components/pricing/pricing-teaser";
import TestimonialsSection from "./components/testimonials/testimonials-section";

export const metadata = generatePageMetadata({
  path: "/",
  appendSiteTitle: false,
  keywords: [
    "Go job orchestration",
    "PostgreSQL job queue",
    "workflow DAG engine",
    "background job processing",
    "run retries and dead letter queue",
    "workflow approvals",
    "AI agent workflows",
    "Strait",
  ],
});

const HOW_TO_STEPS = [
  {
    title: "Define jobs and workflows",
    description:
      "Create job definitions and DAG workflows with dependencies, conditions, retries, and approval gates.",
  },
  {
    title: "Trigger and execute",
    description:
      "Trigger runs through API or CLI. Workers claim runs from PostgreSQL using SKIP LOCKED and execute safely in parallel.",
  },
  {
    title: "Observe and control",
    description:
      "Track run state, events, and usage in real time. Replay failed runs, inspect debug bundles, and enforce cost budgets.",
  },
];

const LandingPage = () => {
  const organizationSchema = getOrganizationSchema();
  const webSiteSchema = getWebSiteSchema();
  const softwareAppSchema = getSoftwareApplicationSchema();
  const howToSchema = getHowToSchema(HOW_TO_STEPS);

  return (
    <>
      <JsonLd data={organizationSchema} />
      <JsonLd data={webSiteSchema} />
      <JsonLd data={softwareAppSchema} />
      {howToSchema ? <JsonLd data={howToSchema} /> : null}
      <Hero />
      <InteractiveDemo />
      <ProblemSection />

      <Suspense
        fallback={
          <div className="mx-auto w-full max-w-[1600px] px-4 py-20 sm:px-6 sm:py-28 lg:px-8">
            <div className="space-y-4">
              <div className="mx-auto h-4 w-32 animate-pulse rounded bg-muted/20" />
              <div className="mx-auto h-8 w-64 animate-pulse rounded bg-muted/20" />
              <div className="mt-8 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div
                    className="h-48 animate-pulse rounded-xl bg-muted/20"
                    key={`how-skeleton-${String(i)}`}
                  />
                ))}
              </div>
            </div>
          </div>
        }
      >
        <HowItWorks />
      </Suspense>

      <ProductShowcase />
      <WhyStrait />

      <InterviewShowcase />
      <StylesShowcase />
      <WritingToolkitShowcase />
      <ComparisonSection />

      <Suspense
        fallback={
          <div className="mx-auto w-full max-w-[1600px] px-4 py-20 sm:px-6 sm:py-28 lg:px-8">
            <div className="space-y-4">
              <div className="mx-auto h-4 w-28 animate-pulse rounded bg-muted/20" />
              <div className="mx-auto h-8 w-72 animate-pulse rounded bg-muted/20" />
              <div className="mt-8 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div
                    className="h-56 animate-pulse rounded-xl bg-muted/20"
                    key={`audience-skeleton-${String(i)}`}
                  />
                ))}
              </div>
            </div>
          </div>
        }
      >
        <AudienceSection />
      </Suspense>
      <Suspense
        fallback={
          <div className="mx-auto w-full max-w-[1600px] px-4 py-20 sm:px-6 sm:py-28 lg:px-8">
            <div className="space-y-4">
              <div className="mx-auto h-4 w-28 animate-pulse rounded bg-muted/20" />
              <div className="mx-auto h-8 w-64 animate-pulse rounded bg-muted/20" />
              <div className="mt-8 grid gap-6 sm:grid-cols-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div
                    className="h-40 animate-pulse rounded-xl bg-muted/20"
                    key={`testimonial-skeleton-${String(i)}`}
                  />
                ))}
              </div>
            </div>
          </div>
        }
      >
        <TestimonialsSection />
      </Suspense>
      <PricingTeaser />
      <CTA />
    </>
  );
};

export default LandingPage;
