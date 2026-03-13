import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Image from "next/image";
import Link from "next/link";
import { memo } from "react";
import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { dashboardHref } from "@/lib/urls.ts";
import MobileNav from "./header-mobile-nav.tsx";
import { NAV_LINKS } from "./nav-links.ts";

const Logo = memo(() => (
  <Link className="flex items-center space-x-2" href="/">
    <span className="sr-only">{siteConfig.name}</span>
    <Image
      alt={siteConfig.logo.alt}
      className="h-8 w-auto"
      height={siteConfig.logo.height}
      src={siteConfig.logo.src}
      width={siteConfig.logo.width}
    />
  </Link>
));
Logo.displayName = "Logo";

const Header = () => {
  return (
    <header className="fixed inset-x-0 top-0 z-50 border-border/40 border-b bg-background/5 backdrop-blur-md">
      <Shell variant="wide">
        <nav className="py-3">
          <div className="flex h-12 items-center justify-between">
            <Logo />

            <div className="hidden items-center gap-6 md:flex">
              {NAV_LINKS.map((link) => (
                <Button
                  key={link.label}
                  render={<Link href={link.href} />}
                  size="default"
                  variant="ghost"
                >
                  {link.label}
                </Button>
              ))}
            </div>

            <div className="hidden items-center gap-3 md:flex">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                size="default"
                variant="ghost"
              >
                Sign in
              </Button>
              <Button
                className="transition-shadow duration-300"
                render={<Link href={dashboardHref("/login")} />}
                size="default"
                variant="gradient"
              >
                Run your first job
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>

            <MobileNav />
          </div>
        </nav>
      </Shell>
    </header>
  );
};

export default memo(Header);
