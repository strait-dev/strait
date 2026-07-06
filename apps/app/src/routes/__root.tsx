/// <reference types="vite/client" />

import { Toaster } from "@strait/ui/components/toast";
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
import { getSession } from "@/lib/auth-handler";
import { captureException } from "@/lib/sentry";
import { seo, siteStructuredData } from "@/lib/seo";
import css from "@/styles.css?url";

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
      ...seo(),
      { name: "application-name", content: "Strait" },
      { name: "apple-mobile-web-app-title", content: "Strait" },
      { name: "apple-mobile-web-app-capable", content: "yes" },
      { name: "mobile-web-app-capable", content: "yes" },
      {
        name: "apple-mobile-web-app-status-bar-style",
        content: "black-translucent",
      },
      { name: "format-detection", content: "telephone=no" },
      { name: "theme-color", content: "#ffffff" },
      { "script:ld+json": siteStructuredData() },
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
      <Outlet />
    </RootDocument>
  );
}

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html
      className="light min-h-dvh bg-background antialiased"
      lang="en"
      suppressHydrationWarning
    >
      <head>
        <HeadContent />
      </head>
      <body
        className="h-full bg-background text-foreground selection:bg-foreground selection:text-background"
        suppressHydrationWarning
      >
        <ThemeProvider
          attribute="class"
          defaultTheme="light"
          disableTransitionOnChange
          enableColorScheme={false}
          enableSystem={false}
          scriptProps={{ async: true }}
          storageKey="strait-theme"
          themes={["light", "dark"]}
        >
          {children}
          <Toaster position="bottom-right" />
          {import.meta.env.DEV &&
            import.meta.env.VITE_DISABLE_DEVTOOLS !== "1" && (
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
        </ThemeProvider>
        <Scripts />
      </body>
    </html>
  );
}
