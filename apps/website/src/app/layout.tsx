import { GoogleTagManager } from "@next/third-parties/google";
import { GeistSans } from "geist/font/sans";
import type { Metadata, Viewport } from "next";
import { Toaster } from "sonner";

import "@strait/ui/globals.css";

import { cn } from "@strait/ui/utils";
import NextThemeProvider from "@/components/providers/NextThemeProvider/next-theme-provider";
import { siteConfig } from "@/config/site";

export const metadata: Metadata = {
  metadataBase: new URL(siteConfig.url || "https://trystrait.ai"),
  title: {
    default: siteConfig.title,
    template: `%s — ${siteConfig.name}`,
  },
  description: siteConfig.description,
  keywords: siteConfig.metadata.keywords,
  openGraph: siteConfig.openGraph,
  twitter: siteConfig.twitter,
  icons: siteConfig.icons,
  manifest: siteConfig.manifest,
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
      "max-video-preview": -1,
    },
  },
};

export const viewport: Viewport = {
  initialScale: 1,
  width: "device-width",
  viewportFit: "cover",
};

type Props = {
  children: React.ReactNode;
};

const Layout = ({ children }: Props) => {
  return (
    <html
      className={cn(
        "min-h-screen bg-background antialiased",
        GeistSans.className
      )}
      data-accent={process.env.NEXT_PUBLIC_WEBSITE_ACCENT ?? "teal"}
      lang="en-US"
      suppressHydrationWarning
    >
      <body className="selection:bg-primary selection:text-primary-foreground">
        <NextThemeProvider>{children}</NextThemeProvider>
        <Toaster />
        <GoogleTagManager
          gtmId={process.env.NEXT_PUBLIC_GOOGLE_TAG_MANAGER_ID as string}
        />
      </body>
    </html>
  );
};

export default Layout;
