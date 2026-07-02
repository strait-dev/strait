import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import { MoonIcon, SunIcon } from "@/lib/icons";

const ThemeToggle = () => {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

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
      {mounted && theme === "dark" ? (
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
