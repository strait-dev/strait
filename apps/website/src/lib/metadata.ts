import type { Metadata } from "next";
import { siteConfig } from "@/config/site";

type ArticleMetadata = {
  publishedTime: string;
  modifiedTime?: string;
  authors?: string[];
  section?: string;
  tags?: string[];
};

type GenerateMetadataProps = {
  title?: string;
  description?: string;
  path: string;
  noIndex?: boolean;
  ogImage?: string;
  appendSiteTitle?: boolean;
  keywords?: string[];
  siteName?: string;
  locale?: string;
  ogTitle?: string;
  ogDescription?: string;
  twitterTitle?: string;
  twitterDescription?: string;
  article?: ArticleMetadata;
  canonical?: string;
};

export function generateMetadata({
  title,
  description,
  path,
  noIndex = false,
  ogImage,
  appendSiteTitle = true,
  keywords,
  siteName,
  locale,
  ogTitle,
  ogDescription,
  twitterTitle,
  twitterDescription,
  article,
  canonical,
}: GenerateMetadataProps): Metadata {
  const displayTitle = (() => {
    if (!title) {
      return siteConfig.title;
    }
    if (appendSiteTitle) {
      return `${title} — ${siteConfig.title}`;
    }
    return title;
  })();

  const displayDescription = description ?? siteConfig.description;
  const url = `${siteConfig.url}${path}`;
  const displayImage = ogImage ?? siteConfig.ogImage;
  const displayKeywords = keywords ?? siteConfig.metadata.keywords;
  const displaySiteName =
    siteName ??
    siteConfig.metadata.siteName ??
    siteConfig.openGraph?.siteName ??
    siteConfig.title;
  const displayLocale =
    locale ?? siteConfig.metadata.locale ?? siteConfig.openGraph?.locale;

  const displayOgTitle = ogTitle ?? displayTitle;
  const displayOgDescription = ogDescription ?? displayDescription;
  const displayTwitterTitle = twitterTitle ?? displayOgTitle;
  const displayTwitterDescription = twitterDescription ?? displayOgDescription;

  const openGraphBase = {
    ...siteConfig.openGraph,
    title: displayOgTitle,
    description: displayOgDescription,
    url,
    images: [
      {
        url: displayImage,
        width: 1200,
        height: 630,
        alt: displayOgTitle,
      },
    ],
    siteName: displaySiteName,
    locale: displayLocale,
  };

  const openGraph = article
    ? {
        ...openGraphBase,
        type: "article" as const,
        publishedTime: article.publishedTime,
        ...(article.modifiedTime && { modifiedTime: article.modifiedTime }),
        ...(article.authors && { authors: article.authors }),
        ...(article.section && { section: article.section }),
        ...(article.tags && { tags: article.tags }),
      }
    : openGraphBase;

  const displayCanonical = canonical ?? url;

  return {
    title: displayTitle,
    description: displayDescription,
    keywords: displayKeywords,
    metadataBase: new URL(siteConfig.url),
    alternates: {
      canonical: displayCanonical,
    },
    openGraph,
    twitter: {
      ...siteConfig.twitter,
      title: displayTwitterTitle,
      description: displayTwitterDescription,
      images: [displayImage],
    },
    icons: siteConfig.icons,
    manifest: siteConfig.manifest,
    robots: {
      index: !noIndex,
      follow: !noIndex,
    },
  };
}
