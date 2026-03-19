import dynamic from "next/dynamic";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import {
  getHowToSchema,
  getOrganizationSchema,
  getSoftwareApplicationSchema,
  getWebSiteSchema,
  JsonLd,
} from "@/lib/structured-data.tsx";
import Hero from "./components/common/hero/hero.tsx";
import LogoWall from "./components/logos/logo-wall.tsx";
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
const SocialProofSection = dynamic(
  () => import("./components/testimonials/social-proof-section.tsx")
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
    "AI agent orchestration",
    "managed execution",
    "Fly Machines",
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
      "Track run state, costs, and health scores in real time. Scale with managed container execution on Fly Machines.",
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
      <LogoWall />
      <FeatureBentoGrid />
      <CodeExampleSection />
      <ComparisonSection />
      <PricingTeaser />
      <SocialProofSection />
      <CTA />
    </>
  );
};

export default LandingPage;
