import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";

type TabProps = {
  text: string;
  selected: boolean;
  setSelected: () => void;
  discount?: boolean;
};

const PricingTab = ({
  text,
  selected,
  setSelected,
  discount = false,
}: TabProps) => (
  <button
    className={cn(
      "relative w-fit px-3 py-1.5 font-semibold text-sm capitalize",
      "text-foreground transition-colors",
      "rounded-custom",
      !!discount && "flex items-center justify-center gap-2.5",
      selected
        ? "bg-primary text-primary-foreground shadow-sm"
        : "hover:bg-muted/50"
    )}
    onClick={setSelected}
    type="button"
  >
    <span className="relative z-10">{text}</span>
    {discount ? (
      <Badge
        className={cn(
          "relative z-10 whitespace-nowrap shadow-none",
          !!selected && "bg-primary-foreground/10 text-primary-foreground"
        )}
        variant="success-light"
      >
        Save 17%
      </Badge>
    ) : null}
  </button>
);

export default PricingTab;
