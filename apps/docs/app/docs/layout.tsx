import type { ReactNode } from "react";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { source } from "@/lib/source";

type Props = {
  children: ReactNode;
};

const GITHUB_REPO = "https://github.com/leonardomso/strait";

const Layout = ({ children }: Props) => {
  return (
    <DocsLayout
      tree={source.pageTree}
      nav={{
        title: "Strait Docs",
        url: "/",
      }}
      links={[
        {
          text: "GitHub",
          url: GITHUB_REPO,
          external: true,
        },
      ]}
      githubUrl={GITHUB_REPO}
      sidebar={{
        tabs: [
          {
            title: "Getting Started",
            url: "/docs/getting-started",
          },
          {
            title: "Concepts",
            url: "/docs/concepts",
          },
          {
            title: "SDKs",
            url: "/docs/sdks",
          },
          {
            title: "Integrations",
            url: "/docs/integrations",
          },
          {
            title: "AI Agents",
            url: "/docs/ai",
          },
          {
            title: "API Reference",
            url: "/docs/api-reference",
          },
          {
            title: "CLI",
            url: "/docs/cli",
          },
          {
            title: "Guides",
            url: "/docs/guides",
          },
          {
            title: "Operations",
            url: "/docs/operations",
          },
          {
            title: "Development",
            url: "/docs/development",
          },
        ],
      }}
    >
      {children}
    </DocsLayout>
  );
};

export default Layout;
