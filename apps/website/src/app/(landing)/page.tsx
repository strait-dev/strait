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
import PricingTeaser from "./components/pricing/pricing-teaser.tsx";

const FeatureBentoGrid = dynamic(
  () => import("@/components/landing/feature-bento-grid.tsx")
);
const CodeExampleSection = dynamic(
  () => import("./components/code-examples/code-example-section.tsx")
);
const ComparisonSection = dynamic(
  () => import("./components/comparison/comparison-section.tsx")
);
const ProblemSection = dynamic(
  () => import("./components/common/hero/problem-section.tsx")
);
const CTA = dynamic(() => import("./components/common/cta/cta.tsx"));
const PricingComparison = dynamic(
  () => import("@/components/pricing/pricing-comparison.tsx")
);
const PricingFaq = dynamic(
  () => import("@/components/pricing/pricing-faq.tsx")
);

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
    "AI agent orchestration",
    "managed execution",
    "open source job queue",
    "Strait",
  ],
});

const HOW_TO_STEPS = [
  {
    title: "Install the SDK and define a job",
    description:
      "Install the Strait SDK in TypeScript, Python, Go, Ruby, or Rust. Define job handlers with retries, backoff, and cost budgets.",
  },
  {
    title: "Create workflows and trigger runs",
    description:
      "Build workflow DAGs with step dependencies, conditions, and approval gates. Trigger runs via API, SDK, or schedule.",
  },
  {
    title: "Monitor, observe, and scale",
    description:
      "Track run state, costs, and health scores in real time. Scale with managed container execution across regions.",
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
      <Suspense fallback={null}>
        <ProblemSection />
      </Suspense>
      <Suspense fallback={null}>
        <FeatureBentoGrid />
      </Suspense>
      <div className="mx-auto h-px w-full max-w-[1600px] bg-border/40" />
      <Suspense fallback={null}>
        <CodeExampleSection />
      </Suspense>
      <div className="mx-auto h-px w-full max-w-[1600px] bg-border/40" />
      <Suspense fallback={null}>
        <ComparisonSection />
      </Suspense>
      <div className="mx-auto h-px w-full max-w-[1600px] bg-border/40" />
      <PricingTeaser />
      <Suspense fallback={null}>
        <PricingComparison />
      </Suspense>
      <Suspense fallback={null}>
        <PricingFaq />
      </Suspense>
      <Suspense fallback={null}>
        <CTA />
      </Suspense>
    </>
  );
};

export default LandingPage;
