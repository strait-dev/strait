import { Toolbar } from "basehub/next-toolbar";
import type { ReactNode } from "react";

type Props = {
  children: ReactNode;
};

const Layout = ({ children }: Props) => (
  <>
    {children}
    <Toolbar />
  </>
);

export default Layout;
