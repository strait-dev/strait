"use client";

import { TooltipProvider } from "@strait/ui/components/tooltip";
import { ThemeProvider } from "next-themes";

type Props = {
  children: React.ReactNode;
};

const NextThemeProvider = ({ children }: Props) => (
  <ThemeProvider
    attribute="class"
    defaultTheme="dark"
    disableTransitionOnChange
    enableColorScheme={false}
    enableSystem={false}
    themes={["light", "dark"]}
  >
    <TooltipProvider>{children}</TooltipProvider>
  </ThemeProvider>
);

export default NextThemeProvider;
