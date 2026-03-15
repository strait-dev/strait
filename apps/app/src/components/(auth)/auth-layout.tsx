import type { ReactNode } from "react";

type AuthLayoutProps = {
  children: ReactNode;
  title: string;
};

export const AuthLayout = ({ children, title }: AuthLayoutProps) => {
  return (
    <div className="flex min-h-dvh w-full items-center justify-center bg-background">
      <div className="w-full max-w-[450px] overflow-hidden rounded-custom border border-border/50 bg-background shadow-sm">
        <div className="flex flex-col gap-4 p-8">
          <div className="flex flex-col items-center gap-2">
            <div className="mb-1">
              <img
                alt="Strait Logo"
                className="h-8 w-auto"
                height={32}
                loading="eager"
                src="/strait.svg"
                width={32}
              />
            </div>
            <h1 className="font-normal text-secondary-foreground text-xl tracking-tight">
              {title}
            </h1>
          </div>
          {children}
        </div>
      </div>
    </div>
  );
};
