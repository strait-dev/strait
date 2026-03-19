import { PLANS } from "@strait/billing/products";
import { siteConfig } from "@/config/site.ts";

const BASE_URL = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
const LOGO_URL = `${BASE_URL}/android-chrome-512x512.png`;

type BreadcrumbItem = {
  name: string;
  url: string;
};

type ArticleAuthor = {
  name: string;
  url?: string;
  image?: string;
};

type ArticleData = {
  headline: string;
  description: string;
  image: string;
  datePublished: string;
  dateModified?: string;
  authors: ArticleAuthor[];
  url: string;
  section?: string;
  keywords?: string[];
  wordCount?: number;
};

type PersonData = {
  name: string;
  url: string;
  image?: string;
  jobTitle?: string;
  worksFor?: string;
  description?: string;
  sameAs?: string[];
};

export function getOrganizationSchema() {
  return {
    "@context": "https://schema.org",
    "@type": "Organization",
    "@id": `${BASE_URL}/#organization`,
    name: "Strait",
    url: BASE_URL,
    logo: {
      "@type": "ImageObject",
      url: LOGO_URL,
      width: 512,
      height: 512,
    },
    description: siteConfig.description,
    sameAs: [
      siteConfig.links.twitter,
      siteConfig.links.linkedin,
      siteConfig.links.instagram,
      siteConfig.links.github,
    ].filter(Boolean),
  };
}

export function getWebSiteSchema() {
  return {
    "@context": "https://schema.org",
    "@type": "WebSite",
    "@id": `${BASE_URL}/#website`,
    name: "Strait",
    url: BASE_URL,
    description: siteConfig.description,
    publisher: {
      "@id": `${BASE_URL}/#organization`,
    },
    potentialAction: {
      "@type": "SearchAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: `${BASE_URL}/blog?search={search_term_string}`,
      },
      "query-input": "required name=search_term_string",
    },
  };
}

export function getBlogPostingSchema(article: ArticleData) {
  const authors = article.authors.map((author) => ({
    "@type": "Person" as const,
    name: author.name,
    ...(author.url && { url: author.url }),
    ...(author.image && {
      image: {
        "@type": "ImageObject" as const,
        url: author.image,
      },
    }),
  }));

  return {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    "@id": `${article.url}#article`,
    headline: article.headline,
    description: article.description,
    image: {
      "@type": "ImageObject",
      url: article.image,
      width: 1200,
      height: 630,
    },
    datePublished: article.datePublished,
    ...(article.dateModified && { dateModified: article.dateModified }),
    author: authors.length === 1 ? authors[0] : authors,
    publisher: {
      "@type": "Organization",
      "@id": `${BASE_URL}/#organization`,
      name: "Strait",
      logo: {
        "@type": "ImageObject",
        url: LOGO_URL,
        width: 512,
        height: 512,
      },
    },
    mainEntityOfPage: {
      "@type": "WebPage",
      "@id": article.url,
    },
    isPartOf: {
      "@id": `${BASE_URL}/#website`,
    },
    ...(article.section && { articleSection: article.section }),
    ...(article.keywords &&
      article.keywords.length > 0 && { keywords: article.keywords.join(", ") }),
    ...(article.wordCount && { wordCount: article.wordCount }),
    inLanguage: "en-US",
  };
}

export function getBreadcrumbSchema(items: BreadcrumbItem[]) {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: items.map((item, index) => ({
      "@type": "ListItem",
      position: index + 1,
      name: item.name,
      item: item.url,
    })),
  };
}

export function getPersonSchema(person: PersonData) {
  return {
    "@context": "https://schema.org",
    "@type": "Person",
    "@id": `${person.url}#person`,
    name: person.name,
    url: person.url,
    ...(person.image && {
      image: {
        "@type": "ImageObject",
        url: person.image,
      },
    }),
    ...(person.jobTitle && { jobTitle: person.jobTitle }),
    ...(person.worksFor && {
      worksFor: {
        "@type": "Organization",
        "@id": `${BASE_URL}/#organization`,
        name: person.worksFor,
      },
    }),
    ...(person.description && { description: person.description }),
    ...(person.sameAs && person.sameAs.length > 0 && { sameAs: person.sameAs }),
  };
}

export function getCollectionPageSchema(options: {
  name: string;
  description: string;
  url: string;
}) {
  return {
    "@context": "https://schema.org",
    "@type": "CollectionPage",
    "@id": `${options.url}#webpage`,
    name: options.name,
    description: options.description,
    url: options.url,
    isPartOf: {
      "@id": `${BASE_URL}/#website`,
    },
    about: {
      "@id": `${BASE_URL}/#organization`,
    },
    inLanguage: "en-US",
  };
}

export function getSoftwareApplicationSchema() {
  const offers = [
    {
      "@type": "Offer" as const,
      name: "Personal",
      price: (PLANS.personal.prices.monthly / 100).toFixed(2),
      priceCurrency: "USD",
      priceValidUntil: new Date(
        Date.now() + 365 * 24 * 60 * 60 * 1000
      ).toISOString(),
      availability: "https://schema.org/InStock",
      url: `${BASE_URL}/pricing`,
      description: PLANS.personal.description,
    },
    {
      "@type": "Offer" as const,
      name: "Pro",
      price: (PLANS.pro.prices.monthly / 100).toFixed(2),
      priceCurrency: "USD",
      priceValidUntil: new Date(
        Date.now() + 365 * 24 * 60 * 60 * 1000
      ).toISOString(),
      availability: "https://schema.org/InStock",
      url: `${BASE_URL}/pricing`,
      description: PLANS.pro.description,
    },
  ];

  return {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    "@id": `${BASE_URL}/#software`,
    name: "Strait",
    description: siteConfig.description,
    url: BASE_URL,
    applicationCategory: "ProductivityApplication",
    operatingSystem: "Web",
    browserRequirements:
      "Requires Chrome 90+, Firefox 88+, Safari 14+, or Edge 90+",
    softwareVersion: "2026.1",
    author: {
      "@id": `${BASE_URL}/#organization`,
    },
    provider: {
      "@id": `${BASE_URL}/#organization`,
    },
    offers,
    featureList: [
      "Job orchestration with 13-state lifecycle",
      "Workflow DAG engine with approval gates",
      "Managed container execution on Fly Machines",
      "AI agent platform with cost tracking",
      "5 language SDKs (TypeScript, Python, Go, Ruby, Rust)",
      "Built-in observability with OpenTelemetry",
      "Multi-region deployment with warm pools",
      "Retry strategies with exponential backoff",
      "Dead letter queue and replay",
      "Real-time streaming and health scores",
    ],
    screenshot: {
      "@type": "ImageObject",
      url: `${BASE_URL}/og.png`,
      width: 1200,
      height: 630,
    },
  };
}

type FAQItem = {
  question: string;
  answer: string | null;
};

export function getFAQPageSchema(items: FAQItem[]) {
  // Filter out items with null or empty answers
  const validItems = items.filter(
    (item) => item.answer && item.answer.trim().length > 0
  );

  // Return null if fewer than 3 valid Q&A pairs (Google requirement)
  if (validItems.length < 3) {
    return null;
  }

  return {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    "@id": `${BASE_URL}/pricing#faq`,
    mainEntity: validItems.map((item) => ({
      "@type": "Question",
      name: item.question,
      acceptedAnswer: {
        "@type": "Answer",
        text: item.answer,
      },
    })),
  };
}

type ProductSchemaInput = {
  name: string;
  description: string;
  price: number; // in cents
  slug: string;
};

export function getProductSchema(plan: ProductSchemaInput) {
  return {
    "@context": "https://schema.org",
    "@type": "Product",
    "@id": `${BASE_URL}/pricing#${plan.slug}`,
    name: `Strait ${plan.name}`,
    description: plan.description,
    brand: {
      "@id": `${BASE_URL}/#organization`,
    },
    offers: {
      "@type": "Offer",
      price: (plan.price / 100).toFixed(2),
      priceCurrency: "USD",
      priceValidUntil: new Date(
        Date.now() + 365 * 24 * 60 * 60 * 1000
      ).toISOString(),
      availability: "https://schema.org/InStock",
      url: `${BASE_URL}/pricing`,
      seller: {
        "@id": `${BASE_URL}/#organization`,
      },
    },
    category: "Workflow Orchestration Software",
  };
}

type HowToStep = {
  title: string;
  description: string;
};

export function getHowToSchema(steps: HowToStep[]) {
  if (steps.length === 0) {
    return null;
  }

  return {
    "@context": "https://schema.org",
    "@type": "HowTo",
    "@id": `${BASE_URL}/#howto`,
    name: "How to Get Started with Strait",
    description:
      "Learn how to get started with Strait job orchestration in three simple steps.",
    totalTime: "PT10M",
    estimatedCost: {
      "@type": "MonetaryAmount",
      currency: "USD",
      value: "0",
    },
    step: steps.map((step, index) => ({
      "@type": "HowToStep",
      position: index + 1,
      name: step.title,
      text: step.description,
      url: `${BASE_URL}/#step-${index + 1}`,
    })),
    tool: [
      {
        "@type": "HowToTool",
        name: "Web browser",
      },
      {
        "@type": "HowToTool",
        name: "Strait API or CLI",
      },
    ],
  };
}

export function getPricingProductsSchema() {
  const plans: ProductSchemaInput[] = [
    {
      name: "Personal",
      description: PLANS.personal.description,
      price: PLANS.personal.prices.monthly,
      slug: "personal",
    },
    {
      name: "Pro",
      description: PLANS.pro.description,
      price: PLANS.pro.prices.monthly,
      slug: "pro",
    },
  ];

  return plans.map((plan) => getProductSchema(plan));
}

export function JsonLd({
  data,
}: {
  data: Record<string, unknown>;
}): React.JSX.Element {
  return (
    <script
      // biome-ignore lint/security/noDangerouslySetInnerHtml: JSON-LD requires dangerouslySetInnerHTML for structured data
      dangerouslySetInnerHTML={{ __html: JSON.stringify(data) }}
      type="application/ld+json"
    />
  );
}

export function JsonLdMultiple({
  data,
}: {
  data: Record<string, unknown>[];
}): React.JSX.Element {
  return (
    <script
      // biome-ignore lint/security/noDangerouslySetInnerHtml: JSON-LD requires dangerouslySetInnerHTML for structured data
      dangerouslySetInnerHTML={{ __html: JSON.stringify(data) }}
      type="application/ld+json"
    />
  );
}
