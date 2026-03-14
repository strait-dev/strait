import type { ReactNode } from "react";

type AuthLayoutProps = {
  children: ReactNode;
  title: string;
};

export const AuthLayout = ({ children, title }: AuthLayoutProps) => {
  return (
    <div className="relative flex min-h-screen w-full items-center justify-center overflow-hidden bg-gradient-to-br from-primary/20 via-background to-primary/10">
      {/* Base grid pattern */}
      <div
        className="absolute inset-0"
        style={{
          opacity: 0.12,
          backgroundImage: `
            linear-gradient(to right, hsl(var(--primary) / 0.4) 1px, transparent 1px),
            linear-gradient(to bottom, hsl(var(--primary) / 0.4) 1px, transparent 1px)
          `,
          backgroundSize: "32px 32px",
          backgroundPosition: "center center",
          maskImage:
            "radial-gradient(ellipse 70% 70% at 50% 0%, black, transparent)",
        }}
      />

      {/* Larger squares overlay */}
      <div
        className="absolute inset-0"
        style={{
          opacity: 0.1,
          backgroundImage: `
            linear-gradient(to right, hsl(var(--primary) / 0.5) 1.5px, transparent 1.5px),
            linear-gradient(to bottom, hsl(var(--primary) / 0.5) 1.5px, transparent 1.5px)
          `,
          backgroundSize: "128px 128px",
          backgroundPosition: "center center",
          maskImage:
            "radial-gradient(ellipse 70% 70% at 50% 0%, black, transparent)",
        }}
      />

      {/* Radial gradient overlay */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "radial-gradient(circle at 50% -50%, hsl(var(--primary) / 0.08), transparent 75%)",
        }}
      />

      {/* Card */}
      <div className="relative w-full max-w-[450px] overflow-hidden rounded-custom border border-border/50 bg-background shadow-sm">
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
