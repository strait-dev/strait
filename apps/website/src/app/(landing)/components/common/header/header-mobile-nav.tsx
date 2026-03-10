"use client";

import {
  ArrowRight02Icon,
  Cancel01Icon,
  Menu01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import { dashboardHref } from "@/lib/urls";

const NAV_LINKS = [
  { label: "Features", href: "/#features" },
  { label: "How it works", href: "/#how-it-works" },
  { label: "Pricing", href: "/pricing" },
  { label: "Blog", href: "/blog" },
] as const;

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
    <div className="md:hidden">
      <button
        aria-controls="mobile-nav-panel"
        aria-expanded={isOpen}
        aria-label={isOpen ? "Close menu" : "Open menu"}
        className="flex size-10 items-center justify-center rounded-custom text-muted-foreground transition-colors hover:text-foreground"
        onClick={toggle}
        ref={toggleRef}
        type="button"
      >
        <HugeiconsIcon
          className="size-5"
          icon={isOpen ? Cancel01Icon : Menu01Icon}
        />
      </button>

      {isOpen && (
        <div className="absolute top-full right-0 left-0 mt-2 px-4">
          <div
            className="rounded-custom border border-border/40 bg-background/95 p-4 shadow-lg backdrop-blur-md"
            id="mobile-nav-panel"
            ref={dropdownRef}
          >
            <div className="flex flex-col gap-1">
              {NAV_LINKS.map((link) => (
                <Button
                  className="justify-start text-muted-foreground hover:text-foreground"
                  key={link.label}
                  onClick={close}
                  render={<Link href={link.href} />}
                  size="default"
                  variant="ghost"
                >
                  {link.label}
                </Button>
              ))}
            </div>
            <div className="mt-3 flex flex-col gap-2 border-border/40 border-t pt-3">
              <Button
                className="text-muted-foreground hover:text-foreground"
                render={<Link href={dashboardHref("/login")} />}
                size="default"
                variant="ghost"
              >
                Sign in
              </Button>
              <Button
                className="gradient-warm text-white shadow-sm"
                render={<Link href={dashboardHref("/login")} />}
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
