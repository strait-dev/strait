import {
  CheckmarkBadge02Icon,
  DownloadSquare02Icon,
  FileSearchIcon,
  ShieldBlockchainIcon,
} from "@hugeicons/core-free-icons";

import FeatureShowcase from "./feature-showcase";

const PlagiarismDetectionShowcase = () => (
  <FeatureShowcase
    cta={{
      href: "/app",
      label: "Check originality now",
    }}
    description="Scan your content against billions of web pages to ensure originality. Get detailed similarity reports with source citations for complete transparency."
    features={[
      {
        title: "Web-wide content scanning",
        description:
          "Check your writing against online sources to identify potential matches and overlaps.",
        icon: FileSearchIcon,
      },
      {
        title: "Similarity score and insights",
        description:
          "Receive clear percentage scores showing how much of your content matches existing sources.",
        icon: CheckmarkBadge02Icon,
      },
      {
        title: "Source citations and highlights",
        description:
          "See exactly which passages match and where they come from with highlighted references.",
        icon: ShieldBlockchainIcon,
      },
      {
        title: "Exportable plagiarism reports",
        description:
          "Generate professional reports you can share with clients, editors, or academic institutions.",
        icon: DownloadSquare02Icon,
      },
    ]}
    title="Verify originality with confidence"
  />
);

export default PlagiarismDetectionShowcase;
