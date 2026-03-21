import { HugeiconsIcon } from "@hugeicons/react";

type ChartEmptyStateProps = {
  icon: any;
  message: string;
};

const ChartEmptyState = ({ icon, message }: ChartEmptyStateProps) => {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
        <HugeiconsIcon className="size-5 text-muted-foreground" icon={icon} />
      </div>
      <p className="max-w-[200px] text-center text-muted-foreground text-sm leading-snug">
        {message}
      </p>
    </div>
  );
};

export default ChartEmptyState;
