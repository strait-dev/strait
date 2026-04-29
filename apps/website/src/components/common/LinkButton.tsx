import { Button } from "@strait/ui/components/button";
import type { ReactNode } from "react";

interface LinkButtonProps {
  ariaLabel?: string;
  children: ReactNode;
  className?: string;
  href: string;
  rel?: string;
  size?: "default" | "xs" | "sm" | "lg" | "xl";
  target?: string;
  variant?: "default" | "outline" | "ghost";
}

export function LinkButton({
  href,
  variant = "default",
  size = "default",
  className,
  target,
  rel,
  ariaLabel,
  children,
}: LinkButtonProps) {
  return (
    <Button
      className={className}
      render={
        // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
        <a aria-label={ariaLabel} href={href} rel={rel} target={target} />
      }
      size={size}
      variant={variant}
    >
      {children}
    </Button>
  );
}

export default LinkButton;
