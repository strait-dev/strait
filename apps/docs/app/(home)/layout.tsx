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
      }}
      links={[
        { text: "Documentation", url: "/docs/getting-started" },
        { text: "API Reference", url: "/docs/api-reference" },
        { text: "SDKs", url: "/docs/sdks" },
        { text: "CLI", url: "/docs/cli" },
      ]}
    >
      {children}
    </HomeLayout>
  );
};

export default Layout;
