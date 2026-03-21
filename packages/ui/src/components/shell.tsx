import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "../utils/index";

const shellVariants = cva("flex flex-col gap-4", {
  variants: {
    variant: {
      default: "w-full px-3 py-2 sm:px-4",
      centered: "w-full items-center px-3 py-2 sm:px-4",
      fluid: "w-full px-3 py-2 sm:px-4",
    },
  },
  defaultVariants: {
    variant: "default",
  },
});

type ShellProps = React.HTMLAttributes<HTMLDivElement> &
  VariantProps<typeof shellVariants> & {
    className?: string;
    variant?: "default" | "centered" | "fluid";
  };

function Shell({ className, variant, ...props }: ShellProps) {
  return (
    <div className={cn(shellVariants({ variant }), className)} {...props} />
  );
}

export { Shell, type ShellProps };
