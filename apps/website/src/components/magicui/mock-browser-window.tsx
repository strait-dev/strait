import { cn } from "@strait/ui/utils";

type MockBrowserWindowProps = {
  children: React.ReactNode;
  className?: string;
  url?: string;
  actions?: React.ReactNode;
};

const MockBrowserWindow = ({
  children,
  className,
  url,
  actions,
}: MockBrowserWindowProps) => (
  <div
    className={cn(
      "overflow-hidden rounded-2xl border border-border/60 bg-muted/40",
      className
    )}
  >
    <div className="flex items-center justify-between border-border/50 border-b px-4 py-3">
      <div className="flex items-center gap-1.5">
        <span className="size-3 rounded-full bg-[#FF5F57]" />
        <span className="size-3 rounded-full bg-[#FEBC2E]" />
        <span className="size-3 rounded-full bg-[#28C840]" />
        {url && (
          <span className="ml-3 text-muted-foreground/50 text-xs">{url}</span>
        )}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
    {children}
  </div>
);

export default MockBrowserWindow;
