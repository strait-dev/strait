import { Card, CardContent } from "@strait/ui/components/card";
import type { ReactNode } from "react";

type AuthLayoutProps = {
  children: ReactNode;
  title: string;
  description?: string;
};

const AuthLayout = ({ children, title, description }: AuthLayoutProps) => (
  <div className="flex min-h-dvh w-full items-center justify-center">
    <Card className="w-full max-w-[450px] overflow-hidden">
      <CardContent className="flex flex-col gap-4 p-8">
        <div className="flex flex-col items-center gap-2">
          <div className="mb-1">
            <img
              alt="Strait logo"
              className="h-8 w-auto"
              height={32}
              loading="eager"
              src="/strait-logo-black.svg"
              width={32}
            />
          </div>
          <h1 className="text-balance font-normal text-secondary-foreground text-xl tracking-tight">
            {title}
          </h1>
          {description ? (
            <p className="text-pretty text-center text-muted-foreground text-sm">
              {description}
            </p>
          ) : null}
        </div>
        {children}
      </CardContent>
    </Card>
  </div>
);

export default AuthLayout;
