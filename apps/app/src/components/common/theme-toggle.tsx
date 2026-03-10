import { Moon02Icon, Sun01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";

export const ThemeToggle = () => {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  // Avoid hydration mismatch by only rendering after mount
  useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return (
      <Button
        className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
        size="icon"
        variant="outline"
      >
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4"
          icon={Moon02Icon}
        />
        <span className="sr-only">Toggle theme</span>
      </Button>
    );
  }

  const toggleTheme = () => {
    setTheme(theme === "dark" ? "light" : "dark");
  };

  return (
    <Button
      className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
      onClick={toggleTheme}
      size="icon"
      variant="outline"
    >
      {theme === "dark" ? (
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4 transition-all"
          icon={Sun01Icon}
        />
      ) : (
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4 transition-all"
          icon={Moon02Icon}
        />
      )}
      <span className="sr-only">Toggle theme</span>
    </Button>
  );
};
