import { buttonVariants } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { createLink } from "@tanstack/react-router";
import { forwardRef } from "react";

const ButtonLinkComponent = forwardRef<
  HTMLAnchorElement,
  React.AnchorHTMLAttributes<HTMLAnchorElement>
>(({ className, ...props }, ref) => {
  return (
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
});

export const ButtonLink = createLink(ButtonLinkComponent);
