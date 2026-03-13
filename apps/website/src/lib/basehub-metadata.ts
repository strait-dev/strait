import React from "react";

export type PageMetadataContent = {
  title?: string;
  description?: string;
  keywords?: string[];
  siteName?: string;
  locale?: string;
  themeColor?: string;
  ogImage?: string;
};

export const fetchPageMetadata = React.cache(
  (_slug: string): PageMetadataContent | null => {
    return null;
  }
);
