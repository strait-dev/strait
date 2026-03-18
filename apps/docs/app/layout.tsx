import { GoogleTagManager } from "@next/third-parties/google";
import { RootProvider } from "fumadocs-ui/provider/next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import type { Metadata, Viewport } from "next";

import "./global.css";

export const metadata: Metadata = {
  metadataBase: new URL("https://docs.strait.dev"),
  title: {
    default: "Strait Docs",
    template: "%s — Strait Docs",
  },
  description:
    "Documentation for Strait, the background job orchestration platform.",
  icons: {
    icon: [
      { url: "/favicon-32x32.png", sizes: "32x32", type: "image/png" },
      { url: "/favicon-16x16.png", sizes: "16x16", type: "image/png" },
    ],
    shortcut: "/favicon.ico",
  },
  openGraph: {
    type: "website",
    siteName: "Strait Docs",
    title: "Strait Docs",
    description:
      "Documentation for Strait, the background job orchestration platform.",
    url: "https://docs.strait.dev",
  },
  twitter: {
    card: "summary_large_image",
    title: "Strait Docs",
    description:
      "Documentation for Strait, the background job orchestration platform.",
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
      className={`${GeistSans.variable} ${GeistMono.variable}`}
      lang="en-US"
      suppressHydrationWarning
    >
      <body className="min-h-screen bg-background font-sans text-foreground antialiased">
        <RootProvider>{children}</RootProvider>
        {process.env.NEXT_PUBLIC_GOOGLE_TAG_MANAGER_ID ? (
          <GoogleTagManager
            gtmId={process.env.NEXT_PUBLIC_GOOGLE_TAG_MANAGER_ID}
          />
        ) : null}
      </body>
    </html>
  );
};

export default Layout;
