import { cn } from "../utils/index";

export type InputWithAddonsProps =
  React.InputHTMLAttributes<HTMLInputElement> & {
    leading?: React.ReactNode;
    trailing?: React.ReactNode;
    containerClassName?: string;
  };

function InputWithAddons({
  leading,
  trailing,
  containerClassName,
  className,
  ...props
}: InputWithAddonsProps) {
  return (
    <div
      className={cn(
        "group flex h-9 w-full overflow-hidden rounded-md border border-input bg-input/20 text-sm ring-offset-background focus-within:outline-hidden focus-within:ring-[3px] focus-within:ring-ring/50 focus-within:ring-offset-2 dark:bg-input/30",
        containerClassName
      )}
      data-slot="input-with-addons"
    >
      {leading ? (
        <div
          className="border-input border-r bg-muted px-3 py-2"
          data-slot="leading-addon"
        >
          {leading}
        </div>
      ) : null}
      <input
        className={cn(
          "w-full rounded-md bg-transparent px-3 py-2 placeholder:text-muted-foreground focus:outline-hidden disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
        data-slot="input"
        {...props}
      />
      {trailing ? (
        <div
          className="border-input border-l bg-muted px-3 py-2"
          data-slot="trailing-addon"
        >
          {trailing}
        </div>
      ) : null}
    </div>
  );
}

export { InputWithAddons };
