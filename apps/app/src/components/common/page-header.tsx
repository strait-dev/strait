import type React from "react";

type Props = {
  title: string;
  text: string;
  button?: React.ReactNode;
};

const PageHeader = ({ title, text, button }: Props) => (
  <header className="w-full pb-2">
    <div className="flex flex-col items-start gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div className="flex flex-col justify-start self-start">
        <h1
          className="text-balance font-normal text-2xl text-secondary-foreground tracking-tight"
          data-testid="page-header-title"
        >
          {title}
        </h1>
        <p
          className="whitespace-normal text-pretty text-muted-foreground text-sm"
          data-testid="page-header-text"
        >
          {text}
        </p>
      </div>

      {button}
    </div>
  </header>
);

export default PageHeader;
