import {
  ArrowRight02Icon,
  Cancel01Icon,
  Menu01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useCallback, useEffect, useRef, useState } from "react";

import { dashboardHref } from "@/lib/urls.ts";
import { isNavGroup, NAV_ITEMS, type NavGroup } from "./nav-links.ts";

const MobileNavGroup = ({
  group,
  onNavigate,
}: {
  group: NavGroup;
  onNavigate: () => void;
}) => {
  const [expanded, setExpanded] = useState(false);
  const toggleExpanded = useCallback(() => setExpanded((prev) => !prev), []);

  return (
    <div>
      <button
        aria-expanded={expanded}
        className="flex w-full items-center justify-between rounded-md px-3 py-2 font-medium text-foreground/80 text-sm transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={toggleExpanded}
        type="button"
      >
        {group.label}
        <svg
          className={`size-4 transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {expanded && (
        <div className="mt-1 ml-2 flex flex-col gap-2 border-border/40 border-l pl-3">
          {group.children.map((section) => (
            <div key={section.groupLabel}>
              <p className="mb-1 px-3 font-semibold text-muted-foreground text-xs uppercase tracking-wider">
                {section.groupLabel}
              </p>
              <div className="flex flex-col gap-0.5">
                {section.links.map((link) => (
                  <a
                    className="rounded-md px-3 py-1.5 text-foreground/80 text-sm transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    href={link.href}
                    key={link.href}
                    onClick={onNavigate}
                  >
                    {link.label}
                  </a>
                ))}
              </div>
            </div>
          ))}

          {group.featured && (
            <a
              className="inline-flex items-center gap-1 px-3 py-1.5 font-medium text-primary text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              href={group.featured.href}
              onClick={onNavigate}
            >
              {group.featured.label}
              <HugeiconsIcon className="size-3.5" icon={ArrowRight02Icon} />
            </a>
          )}
        </div>
      )}
    </div>
  );
};

const MobileNav = () => {
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const toggle = useCallback(() => setIsOpen((prev) => !prev), []);
  const close = useCallback(() => setIsOpen(false), []);

  // Click-outside to dismiss
  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const handleClick = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        toggleRef.current &&
        !toggleRef.current.contains(e.target as Node)
      ) {
        close();
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [isOpen, close]);

  // Scroll lock when open
  useEffect(() => {
    document.documentElement.classList.toggle("overflow-hidden", isOpen);
    return () => {
      document.documentElement.classList.remove("overflow-hidden");
    };
  }, [isOpen]);

  // Escape key to dismiss
  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        close();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isOpen, close]);

  return (
    <div className="lg:hidden">
      <Button
        aria-controls="mobile-nav-panel"
        aria-expanded={isOpen}
        aria-label={isOpen ? "Close menu" : "Open menu"}
        onClick={toggle}
        ref={toggleRef}
        size="icon-xl"
        variant="ghost"
      >
        <HugeiconsIcon
          className="size-5"
          icon={isOpen ? Cancel01Icon : Menu01Icon}
        />
      </Button>

      {isOpen && (
        <div className="absolute top-full right-0 left-0 z-50 mt-2 px-4">
          <div
            className="rounded-xl border border-border/40 bg-background/95 p-4 shadow-lg backdrop-blur-md"
            id="mobile-nav-panel"
            ref={dropdownRef}
          >
            <div className="flex flex-col gap-1">
              {NAV_ITEMS.map((item) => {
                if (isNavGroup(item)) {
                  return (
                    <MobileNavGroup
                      group={item}
                      key={item.label}
                      onNavigate={close}
                    />
                  );
                }

                return (
                  <a
                    className="inline-flex h-9 items-center rounded-md px-3 font-medium text-foreground/80 text-sm transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    href={item.href}
                    key={item.label}
                    onClick={close}
                  >
                    {item.label}
                  </a>
                );
              })}
            </div>
            <div className="mt-3 flex flex-col gap-2 border-border/40 border-t pt-3">
              <Button
                // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
                render={<a href={dashboardHref("/login")} />}
                size="default"
                variant="ghost"
              >
                Sign in
              </Button>
              <Button
                // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
                render={<a href={dashboardHref("/login")} />}
                size="default"
              >
                Run your first job
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default MobileNav;
