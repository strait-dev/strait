import Image from "next/image";
import Link from "next/link";

import { siteConfig } from "@/config/site.ts";

const FOOTER_LINKS: Record<string, Array<{ label: string; href: string }>> = {
  Product: [
    { label: "Features", href: "/#features" },
    { label: "Pricing", href: "/pricing" },
    { label: "Blog", href: "/blog" },
  ],
  Resources: [{ label: "How it works", href: "/#how-it-works" }],
  Company: [
    { label: "Twitter", href: siteConfig.links.twitter ?? "#" },
    { label: "GitHub", href: siteConfig.links.github ?? "#" },
  ],
  Legal: [
    { label: "Privacy Policy", href: "/privacy" },
    { label: "Terms of Service", href: "/terms" },
  ],
};

const Footer = () => {
  const year = new Date().getFullYear();

  return (
    <footer className="relative overflow-hidden border-border/40 border-t">
      <div className="relative z-10 py-16 sm:py-20">
        <div className="mx-auto w-full max-w-[1600px] px-4 sm:px-6 lg:px-8">
          <div className="flex flex-col justify-between gap-12 md:flex-row md:gap-16">
            <div className="flex shrink-0 flex-col items-start gap-3">
              <Link
                className="inline-flex items-center transition-opacity hover:opacity-80"
                href="/"
              >
                <span className="sr-only">{siteConfig.name}</span>
                <Image
                  alt={siteConfig.logo.alt}
                  className="h-8 w-auto"
                  height={siteConfig.logo.height}
                  src={siteConfig.logo.src}
                  width={siteConfig.logo.width}
                />
              </Link>
              <p className="max-w-xs text-muted-foreground text-sm leading-relaxed">
                Production-grade job orchestration for teams building reliable
                asynchronous systems.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-8 sm:grid-cols-4">
              {Object.entries(FOOTER_LINKS).map(([group, links]) => (
                <div key={group}>
                  <h3 className="font-semibold text-foreground text-sm">
                    {group}
                  </h3>
                  <ul className="mt-4 flex flex-col gap-2.5">
                    {links.map((link) => (
                      <li key={link.label}>
                        <Link
                          className="text-muted-foreground text-sm transition-all hover:text-foreground hover:[text-decoration-color:var(--primary)] hover:[text-decoration-thickness:1.5px] hover:[text-decoration:underline_wavy] hover:[text-underline-offset:4px]"
                          href={link.href}
                          {...(link.href.startsWith("http")
                            ? { target: "_blank", rel: "noopener noreferrer" }
                            : {})}
                        >
                          {link.label}
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          </div>

          <div className="mt-12 border-border/40 border-t pt-6">
            <p className="text-muted-foreground text-sm">
              &copy; {year} {siteConfig.name}. All rights reserved.
            </p>
          </div>
        </div>
      </div>

      <div
        aria-hidden="true"
        className="pointer-events-none relative -mt-8 select-none overflow-hidden"
        style={{
          maskImage:
            "linear-gradient(to bottom, black 0%, black 50%, transparent 100%)",
          WebkitMaskImage:
            "linear-gradient(to bottom, black 0%, black 50%, transparent 100%)",
        }}
      >
        <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
          <p
            className="w-full text-center text-transparent uppercase leading-[0.8]"
            style={{
              fontSize: "clamp(3rem, 18vw, 19rem)",
              WebkitTextStroke: "1.5px var(--color-foreground)",
              opacity: 0.06,
            }}
          >
            STRAIT
          </p>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
