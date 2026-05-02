import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useCallback, useEffect, useRef, useState } from "react";

import type { NavGroup } from "./nav-links.ts";

type HeaderDropdownProps = {
  group: NavGroup;
};

const HeaderDropdown = ({ group }: HeaderDropdownProps) => {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLElement>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const open = useCallback(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    setIsOpen(true);
  }, []);

  const close = useCallback(() => {
    timeoutRef.current = setTimeout(() => {
      setIsOpen(false);
    }, 150);
  }, []);

  const closeImmediately = useCallback(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    setIsOpen(false);
  }, []);

  // Escape key to dismiss
  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeImmediately();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isOpen, closeImmediately]);

  // Click outside to dismiss
  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const handleClick = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        closeImmediately();
      }
    };

    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [isOpen, closeImmediately]);

  // Cleanup timeout on unmount
  useEffect(
    () => () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    },
    []
  );

  return (
    // biome-ignore lint/a11y/noNoninteractiveElementInteractions: hover/focus handlers needed for mega-menu dropdown behavior
    <nav
      className="relative"
      onBlur={close}
      onFocus={open}
      onMouseEnter={open}
      onMouseLeave={close}
      ref={containerRef}
    >
      <button
        aria-expanded={isOpen}
        aria-haspopup="true"
        className="inline-flex h-9 items-center gap-1 rounded-md px-3 font-medium text-foreground/80 text-sm transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        type="button"
      >
        {group.label}
        <svg
          className={`size-3.5 transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`}
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {isOpen && (
        <div
          className="fade-in zoom-in-95 absolute top-full left-1/2 z-50 mt-2 w-max min-w-[28rem] -translate-x-1/2 animate-in rounded-xl border border-border/40 bg-background/95 p-4 shadow-xl backdrop-blur-md duration-150"
          role="menu"
        >
          <div className="grid auto-cols-fr grid-flow-col gap-6">
            {group.children.map((section) => (
              <div key={section.groupLabel}>
                <p className="mb-2 px-2 font-semibold text-muted-foreground text-xs uppercase tracking-wider">
                  {section.groupLabel}
                </p>
                <ul className="flex flex-col gap-0.5">
                  {section.links.map((link) => (
                    <li key={link.href}>
                      <a
                        className="block rounded-md px-2 py-1.5 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        href={link.href}
                        onClick={closeImmediately}
                        role="menuitem"
                      >
                        <span className="font-medium text-foreground text-sm">
                          {link.label}
                        </span>
                        {link.description && (
                          <span className="block text-muted-foreground text-xs">
                            {link.description}
                          </span>
                        )}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          {group.featured && (
            <div className="mt-3 border-border/40 border-t pt-3">
              <a
                className="inline-flex items-center gap-1 rounded-md px-2 py-1 font-medium text-foreground text-sm transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                href={group.featured.href}
                onClick={closeImmediately}
              >
                {group.featured.label}
                <HugeiconsIcon className="size-3.5" icon={ArrowRight02Icon} />
              </a>
            </div>
          )}
        </div>
      )}
    </nav>
  );
};

export default HeaderDropdown;
