import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@strait/ui/components/tooltip";
import { XCircleIcon } from "@/lib/icons";

type FloatingBarAction = {
  label: string;
  icon: any;
  onClick: () => void;
  variant?: "default" | "outline" | "destructive";
};

type DataTableFloatingBarProps = {
  selectedCount: number;
  onClearSelection: () => void;
  actions: FloatingBarAction[];
};

export const DataTableFloatingBar = ({
  selectedCount,
  onClearSelection,
  actions,
}: DataTableFloatingBarProps) => {
  return (
    <TooltipProvider>
      <div className="flex items-center gap-1.5 rounded-lg border bg-background px-3 py-1.5 shadow-lg">
        <span className="px-1 text-sm tabular-nums">
          {selectedCount} selected
        </span>
        {actions.length > 0 && <div className="h-4 w-px bg-border" />}
        {actions.map((action) => (
          <Tooltip key={action.label}>
            <TooltipTrigger
              render={
                <Button
                  aria-label={action.label}
                  onClick={action.onClick}
                  size="icon"
                  variant={action.variant ?? "outline"}
                />
              }
            >
              <HugeiconsIcon icon={action.icon} size={16} />
            </TooltipTrigger>
            <TooltipContent>{action.label}</TooltipContent>
          </Tooltip>
        ))}
        <div className="h-4 w-px bg-border" />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                aria-label="Clear selection"
                onClick={onClearSelection}
                size="icon"
                variant="ghost"
              />
            }
          >
            <HugeiconsIcon icon={XCircleIcon} size={16} />
          </TooltipTrigger>
          <TooltipContent>Clear selection</TooltipContent>
        </Tooltip>
      </div>
    </TooltipProvider>
  );
};
