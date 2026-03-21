import { HugeiconsIcon } from "@hugeicons/react";

const ConfigRow = ({
  icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) => {
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <div className="flex shrink-0 items-center gap-2">
        <HugeiconsIcon
          className="text-muted-foreground"
          icon={icon}
          size={14}
        />
        <span className="text-muted-foreground">{label}</span>
      </div>
      <span className="truncate font-mono text-xs">{value}</span>
    </div>
  );
};

export default ConfigRow;
