import {
  BookEditIcon,
  Globe02Icon,
  Mail01Icon,
  News01Icon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls";
import FeatureShowcase from "./feature-showcase";
import {
  ContentTypesVisual1,
  ContentTypesVisual2,
  ContentTypesVisual3,
  ContentTypesVisual4,
} from "./visuals/content-types-visual";

const MultiLanguageShowcase = () => (
  <FeatureShowcase
    cta={{
      href: dashboardHref("/login"),
      label: "Explore content types",
    }}
    description="Each content type comes with specialized prompts. The AI adapts its style, structure, and tone based on what you're writing."
    features={[
      {
        title: "Blog posts and articles",
        description:
          "Long-form content with hook-first, story-led, and data-driven angle options built in.",
        icon: BookEditIcon,
      },
      {
        title: "Social media threads",
        description:
          "Twitter threads and LinkedIn posts optimized for engagement with platform-specific formatting.",
        icon: Globe02Icon,
      },
      {
        title: "Emails and newsletters",
        description:
          "Follow-up emails, newsletters, and outreach with tone-matched subject lines and CTAs.",
        icon: Mail01Icon,
      },
      {
        title: "Press releases and ad copy",
        description:
          "Professional press releases and advertising copy with industry-standard structures.",
        icon: News01Icon,
      },
    ]}
    orientation="visual-left"
    title="10 content types with specialized prompts"
    visuals={[
      <ContentTypesVisual1 key="ctv1" />,
      <ContentTypesVisual2 key="ctv2" />,
      <ContentTypesVisual3 key="ctv3" />,
      <ContentTypesVisual4 key="ctv4" />,
    ]}
  />
);

export default MultiLanguageShowcase;
