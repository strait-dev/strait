import type { ReactNode } from "react";
import { HomeLayout } from "fumadocs-ui/layouts/home";

type Props = {
  children: ReactNode;
};

const Layout = ({ children }: Props) => {
  return (
    <HomeLayout
      nav={{
        title: "Strait Docs",
        url: "/",
      }}
      links={[
        { text: "Documentation", url: "/docs/getting-started" },
        { text: "API Reference", url: "/docs/api-reference" },
        { text: "SDKs", url: "/docs/sdks" },
        {
          text: "GitHub",
          url: "https://github.com/leonardomso/strait",
          external: true,
        },
      ]}
      githubUrl="https://github.com/leonardomso/strait"
    >
      {children}
    </HomeLayout>
  );
};

export default Layout;
