/// <reference types="vite/client" />

import { Toaster } from "@strait/ui/components/toast/index";
import css from "@strait/ui/globals.css?url";
import { TanStackDevtools } from "@tanstack/react-devtools";
import type { QueryClient } from "@tanstack/react-query";
import { ReactQueryDevtoolsPanel } from "@tanstack/react-query-devtools";
import {
  createRootRouteWithContext,
  HeadContent,
  Outlet,
  Scripts,
} from "@tanstack/react-router";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";
import { ThemeProvider } from "next-themes";
import { useEffect, useState } from "react";
import { getSession } from "@/lib/auth-handler";
import { captureException } from "@/lib/sentry";

export type AuthUser = {
  id: string;
  name: string;
  email: string;
  emailVerified: boolean;
  image?: string | null;
  defaultOrganizationId?: string;
  activeProjectId?: string;
  onboarded?: boolean;
  twoFactorEnabled?: boolean;
  createdAt: Date;
  updatedAt: Date;
};

export type Session = {
  user: AuthUser;
  session: {
    id: string;
    userId: string;
    token: string;
    expiresAt: Date;
  };
} | null;

export type RouterContext = {
  queryClient: QueryClient;
  isAuthenticated: boolean;
  session: Session;
};

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async () => {
    try {
      const session = await getSession();

      return {
        session,
        isAuthenticated: !!session,
      };
    } catch (error) {
      captureException(error, {
        tags: { location: "root_beforeLoad" },
      });
      return { session: null, isAuthenticated: false };
    }
  },
  head: () => ({
    meta: [
      {
        charSet: "utf-8",
      },
      {
        name: "viewport",
        content: "width=device-width, initial-scale=1",
      },
      {
        title: "Strait",
      },
      {
        name: "description",
        content:
          "Strait is a production-grade job orchestration platform for scheduling, executing, and monitoring distributed workloads.",
      },
      { property: "og:title", content: "Strait" },
      {
        property: "og:description",
        content:
          "Strait is a production-grade job orchestration platform for scheduling, executing, and monitoring distributed workloads.",
      },
      { property: "og:image", content: "/og.png" },
      { property: "og:type", content: "website" },
      { name: "twitter:card", content: "summary_large_image" },
      { name: "twitter:image", content: "/og.png" },
    ],
    links: [
      { rel: "stylesheet", href: css },
      { rel: "preconnect", href: "https://fonts.googleapis.com" },
      { rel: "preconnect", href: "https://fonts.gstatic.com", crossOrigin: "" },
      {
        rel: "stylesheet",
        href: "https://fonts.googleapis.com/css2?family=Geist:wght@100;200;300;400;500;600;700;800;900&family=Geist+Mono:wght@100;200;300;400;500;600;700;800;900&display=swap",
      },
      { rel: "icon", href: "/favicon.ico" },
      {
        rel: "icon",
        type: "image/png",
        sizes: "32x32",
        href: "/favicon-32x32.png",
      },
      {
        rel: "icon",
        type: "image/png",
        sizes: "16x16",
        href: "/favicon-16x16.png",
      },
      {
        rel: "apple-touch-icon",
        sizes: "180x180",
        href: "/apple-touch-icon.png",
      },
      { rel: "manifest", href: "/site.webmanifest" },
    ],
  }),
  component: RootComponent,
});

function RootComponent() {
  return (
    <RootDocument>
      <ThemeProvider
        attribute="class"
        defaultTheme="dark"
        disableTransitionOnChange
        enableColorScheme={false}
        enableSystem={false}
        themes={["light", "dark"]}
      >
        <Outlet />
      </ThemeProvider>
    </RootDocument>
  );
}

function RootDocument({ children }: { children: React.ReactNode }) {
  const [isHydrated, setIsHydrated] = useState(false);
  const enableTanStackDevtools =
    import.meta.env.DEV &&
    import.meta.env.VITE_ENABLE_TANSTACK_DEVTOOLS === "true";

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  return (
    <html
      className="dark min-h-dvh bg-background antialiased"
      lang="en"
      suppressHydrationWarning
    >
      <head>
        <HeadContent />
        <meta
          content="width=device-width, height=device-height, initial-scale=1, minimum-scale=1, maximum-scale=1, user-scalable=no"
          name="viewport"
        />
      </head>
      <body
        className="h-full bg-background text-foreground selection:bg-foreground selection:text-background"
        data-hydrated={isHydrated ? "true" : "false"}
        suppressHydrationWarning
      >
        {children}
        <Toaster position="bottom-right" />
        {enableTanStackDevtools && (
          <TanStackDevtools
            config={{ defaultOpen: false }}
            plugins={[
              {
                name: "Tanstack Query",
                render: <ReactQueryDevtoolsPanel />,
              },
              {
                name: "Tanstack Router",
                render: <TanStackRouterDevtoolsPanel />,
              },
            ]}
          />
        )}
        <Scripts />
      </body>
    </html>
  );
}
