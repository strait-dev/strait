/// <reference types="vite/client" />

import { Toaster } from "@strait/ui/components/toast/index.ts";
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
import { getSession } from "@/lib/auth-server.ts";

export type AuthUser = {
  id: string;
  name: string;
  email: string;
  emailVerified: boolean;
  image?: string | null;
  defaultOrganizationId?: string;
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
    } catch {
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
    ],
    links: [
      { rel: "stylesheet", href: css },
      { rel: "preconnect", href: "https://fonts.googleapis.com" },
      { rel: "preconnect", href: "https://fonts.gstatic.com", crossOrigin: "" },
      {
        rel: "stylesheet",
        href: "https://fonts.googleapis.com/css2?family=Geist:wght@100;200;300;400;500;600;700;800;900&family=Geist+Mono:wght@100;200;300;400;500;600;700;800;900&display=swap",
      },
    ],
  }),
  component: RootComponent,
});

function RootComponent() {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="light"
      disableTransitionOnChange
      enableColorScheme={false}
      enableSystem
      themes={["light", "dark"]}
    >
      <RootDocument>
        <Outlet />
      </RootDocument>
    </ThemeProvider>
  );
}

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html
      className="min-h-screen bg-background antialiased"
      lang="en"
      suppressHydrationWarning
    >
      <head>
        <HeadContent />
        <meta
          content="width=device-width, height=device-height, initial-scale=1, minimum-scale=1, maximum-scale=1, user-scalable=no"
          name="viewport"
        />
        <script
          // biome-ignore lint: dangerouslySetInnerHTML needed for theme initialization
          dangerouslySetInnerHTML={{
            __html: `
              try {
                const theme = localStorage.getItem('theme') || 'light';
                const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
                const effectiveTheme = theme === 'system' ? systemTheme : theme;
                document.documentElement.classList.add(effectiveTheme);
              } catch (e) {}
            `,
          }}
        />
      </head>
      <body
        className="h-full bg-background text-foreground selection:bg-primary selection:text-primary-foreground"
        suppressHydrationWarning
      >
        {children}
        <Toaster position="bottom-right" />
        {import.meta.env.DEV && (
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
