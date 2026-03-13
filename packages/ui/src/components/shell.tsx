import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "../utils/index.ts";

const shellVariants = cva("flex flex-col gap-4", {
  variants: {
    variant: {
      default: "mx-auto w-full max-w-[1800px] px-4 pt-2 sm:px-8 lg:px-20",
      centered:
        "mx-auto w-full max-w-[1800px] items-center px-4 sm:px-8 lg:px-20",
      fluid: "w-full px-4 sm:px-8 lg:px-20",
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
