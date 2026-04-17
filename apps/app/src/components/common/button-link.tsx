import { buttonVariants } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { createLink } from "@tanstack/react-router";
import type { RefObject } from "react";

const ButtonLinkComponent = ({
  className,
  ref,
  ...props
}: React.AnchorHTMLAttributes<HTMLAnchorElement> & {
  ref?: RefObject<HTMLAnchorElement | null>;
}) => (
  <a
    className={cn(
      buttonVariants({ variant: "outline", size: "default" }),
      "w-full",
      className
    )}
    ref={ref}
    {...props}
  />
);

export const ButtonLink = createLink(ButtonLinkComponent);
