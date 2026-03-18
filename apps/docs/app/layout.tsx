import { GoogleTagManager } from "@next/third-parties/google";
import { RootProvider } from "fumadocs-ui/provider/next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import type { Metadata, Viewport } from "next";

import "./global.css";

export const metadata: Metadata = {
  title: {
    default: "Strait Docs",
    template: "%s — Strait Docs",
  },
  description:
    "Documentation for Strait, the background job orchestration platform.",
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
