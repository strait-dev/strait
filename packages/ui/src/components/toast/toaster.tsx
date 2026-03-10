"use client";

import { useTheme } from "next-themes";
import { Toaster as Sonner } from "sonner";

type ToasterProps = React.ComponentProps<typeof Sonner>;

function Toaster({ ...props }: ToasterProps) {
  const { theme = "system" } = useTheme();

  return (
    <Sonner
      className="toaster group"
      position="bottom-right"
      theme={theme as ToasterProps["theme"]}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast: "pointer-events-auto",
        },
      }}
      {...props}
    />
  );
}

export { Toaster };
