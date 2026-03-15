import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { XCircleIcon } from "@/lib/icons";

type FloatingBarAction = {
  label: string;
  icon?: any;
  onClick: () => void;
  variant?: "default" | "outline" | "destructive";
};

type DataTableFloatingBarProps = {
  selectedCount: number;
  onClearSelection: () => void;
  actions: FloatingBarAction[];
};

export function DataTableFloatingBar({
  selectedCount,
  onClearSelection,
  actions,
}: DataTableFloatingBarProps) {
  return (
    <div className="flex items-center gap-2 rounded-lg border bg-background px-4 py-2.5 shadow-lg">
      <span className="text-sm tabular-nums">{selectedCount} selected</span>
      <div className="h-4 w-px bg-border" />
      {actions.map((action) => (
        <Button
          key={action.label}
          onClick={action.onClick}
          size="sm"
          variant={action.variant ?? "outline"}
        >
          {action.icon && (
            <HugeiconsIcon className="mr-1.5" icon={action.icon} size={14} />
          )}
          {action.label}
        </Button>
      ))}
      <div className="h-4 w-px bg-border" />
      <Button
        aria-label="Clear selection"
        onClick={onClearSelection}
        size="icon"
        variant="ghost"
      >
        <HugeiconsIcon icon={XCircleIcon} size={14} />
      </Button>
    </div>
  );
}
