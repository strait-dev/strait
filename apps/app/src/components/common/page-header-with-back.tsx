import { ArrowLeft01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Link } from "@tanstack/react-router";
import type React from "react";

type Props = {
  title: string;
  text: string;
  backHref: string;
  backLabel?: string;
  button?: React.ReactNode;
};

const PageHeaderWithBack = ({
  title,
  text,
  backHref,
  backLabel = "Back",
  button,
}: Props) => {
  return (
    <header className="w-full space-y-4">
      {/* Back Button */}
      <div>
        <Button render={<Link to={backHref} />} variant="ghost">
          <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
          {backLabel}
        </Button>
      </div>

      {/* Page Header */}
      <div className="flex flex-col items-end gap-5 sm:flex-row sm:justify-between">
        <div className="flex flex-col justify-start self-start">
          <h1
            className="font-normal text-secondary-foreground text-xl tracking-tight"
            data-testid="page-header-title"
          >
            {title}
          </h1>
          <p
            className="whitespace-normal text-muted-foreground text-sm"
            data-testid="page-header-text"
          >
            {text}
          </p>
        </div>

        {button}
      </div>
    </header>
  );
};

export default PageHeaderWithBack;
