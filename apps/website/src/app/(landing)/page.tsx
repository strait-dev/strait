import dynamic from "next/dynamic";
import { Suspense } from "react";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import {
  getHowToSchema,
  getOrganizationSchema,
  getSoftwareApplicationSchema,
  getWebSiteSchema,
  JsonLd,
} from "@/lib/structured-data.tsx";
import Hero from "./components/common/hero/hero.tsx";
import ProblemSection from "./components/common/hero/problem-section.tsx";
import HowItWorks from "./components/how-it-works/how-it-works.tsx";
import PricingTeaser from "./components/pricing/pricing-teaser.tsx";

const PipelineDemo = dynamic(
  () => import("@/components/landing/pipeline-demo.tsx")
);
const FeatureBentoGrid = dynamic(
  () => import("@/components/landing/feature-bento-grid.tsx")
);
const WhyStrait = dynamic(
  () => import("./components/benefits/why-polyglot.tsx")
);
const CredibilitySection = dynamic(
  () => import("@/components/landing/credibility-section.tsx")
);
const ComparisonSection = dynamic(
  () => import("./components/comparison/comparison-section.tsx")
);
const CTA = dynamic(() => import("./components/common/cta/cta.tsx"));

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
      "Trigger runs through API or CLI. Workers claim runs from PostgreSQL and execute safely in parallel.",
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
      <PipelineDemo />
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

      <FeatureBentoGrid />
      <WhyStrait />
      <CredibilitySection />
      <ComparisonSection />
      <PricingTeaser />
      <CTA />
    </>
  );
};

export default LandingPage;
