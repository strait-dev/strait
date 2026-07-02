import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useTheme } from "next-themes";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { MoonIcon, SunIcon } from "@/lib/icons";

const ThemeToggle = () => {
  const { theme, setTheme } = useTheme();
  const isHydrated = useIsHydrated();

  const toggleTheme = () => {
    setTheme(theme === "dark" ? "light" : "dark");
  };

  return (
    <Button
      aria-label="Toggle theme"
      className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
      onClick={toggleTheme}
      size="icon"
      variant="outline"
    >
      {isHydrated && theme === "dark" ? (
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4 transition-transform"
          icon={SunIcon}
        />
      ) : (
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4 transition-transform"
          icon={MoonIcon}
        />
      )}
      <span className="sr-only">Toggle theme</span>
    </Button>
  );
};

export default ThemeToggle;
