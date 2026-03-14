import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "../utils/index";

const badgeVariants = cva(
  "relative inline-flex w-fit shrink-0 items-center justify-center whitespace-nowrap rounded-4xl border border-transparent font-medium outline-none transition-shadow focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 [&_svg:not([class*=size-])]:size-3 [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground",
        outline: "border-border bg-transparent dark:bg-input/32",
        secondary: "bg-secondary text-secondary-foreground",
        info: "bg-info text-white",
        success: "bg-success text-white",
        warning: "bg-warning text-white",
        destructive: "bg-destructive text-white",
        invert: "bg-invert text-invert-foreground",
        "primary-light":
          "border-none bg-primary/10 text-primary dark:bg-primary/20",
        "warning-light":
          "border-none bg-warning/10 text-warning dark:bg-warning/20",
        "success-light":
          "border-none bg-success/10 text-success dark:bg-success/20",
        "info-light": "border-none bg-info/10 text-info dark:bg-info/20",
        "destructive-light":
          "border-none bg-destructive/10 text-destructive dark:bg-destructive/20",
        "invert-light":
          "border-none bg-invert/10 text-foreground dark:bg-invert/20",
        "secondary-light":
          "border-none bg-secondary/50 text-secondary-foreground dark:bg-secondary/30",
        "primary-outline":
          "border-border bg-background text-primary dark:bg-input/30",
        "warning-outline":
          "border-border bg-background text-warning dark:bg-input/30",
        "success-outline":
          "border-border bg-background text-success dark:bg-input/30",
        "info-outline":
          "border-border bg-background text-info dark:bg-input/30",
        "destructive-outline":
          "border-border bg-background text-destructive dark:bg-input/30",
        "invert-outline":
          "border-border bg-background text-invert-foreground dark:bg-input/30",
        ghost:
          "hover:bg-muted hover:text-muted-foreground dark:hover:bg-muted/50",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        xs: "h-4 min-w-4 gap-1 px-1 py-0.25 text-[0.6rem] leading-none",
        sm: "h-4.5 min-w-4.5 gap-1 px-1 py-0.25 text-[0.625rem] leading-none",
        default: "h-5 min-w-5 gap-1 px-1.25 py-0.5 text-xs",
        lg: "h-5.5 min-w-5.5 gap-1 px-1.5 py-0.5 text-xs",
        xl: "h-6 min-w-6 gap-1.5 px-2 py-0.75 text-sm",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

interface BadgeProps extends useRender.ComponentProps<"span"> {
  size?: VariantProps<typeof badgeVariants>["size"];
  variant?: VariantProps<typeof badgeVariants>["variant"];
}

function Badge({ className, variant, size, render, ...props }: BadgeProps) {
  const defaultProps = {
    "data-slot": "badge",
    className: cn(badgeVariants({ variant, size, className })),
  };

  return useRender({
    defaultTagName: "span",
    render,
    props: mergeProps<"span">(defaultProps, props),
  });
}

export { Badge, type BadgeProps, badgeVariants };
