export type SiteConfig = {
  name: string;
  title: string;
  description: string;
  url: string;
  ogImage: string;
  logo: {
    src: string;
    alt: string;
    width: number;
    height: number;
  };
  links: {
    twitter: string;
    github?: string;
    linkedin?: string;
    instagram?: string;
  };
  metadata: {
    keywords: string[];
    author: string;
    themeColor?: string;
    locale: string;
    siteName: string;
  };
  openGraph: {
    type: "website" | "article" | "profile";
    locale: string;
    url: string;
    title: string;
    description: string;
    siteName: string;
    images: Array<{
      url: string;
      width?: number;
      height?: number;
      alt?: string;
    }>;
  };
  twitter: {
    card: "summary" | "summary_large_image" | "app" | "player";
    creator?: string;
    images?: string[];
  };
  icons: {
    icon: string;
    shortcut: string;
    apple: string;
  };
  manifest: string;
};
