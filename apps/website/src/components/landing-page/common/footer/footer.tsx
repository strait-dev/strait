import { siteConfig } from "@/config/site.ts";

const FOOTER_LINKS: Record<string, Array<{ label: string; href: string }>> = {
  Platform: [
    { label: "PostgreSQL Queue", href: "/features/postgresql-queue" },
    { label: "Workflow DAGs", href: "/features/workflow-dags" },
    { label: "Retries & DLQ", href: "/features/retries-dlq" },
    { label: "All Features", href: "/features" },
  ],
  Solutions: [
    { label: "AI Agent Workflows", href: "/use-cases/ai-agent-workflows" },
    {
      label: "Background Processing",
      href: "/use-cases/background-processing",
    },
    { label: "vs Temporal", href: "/compare/temporal" },
    { label: "vs Inngest", href: "/compare/inngest" },
    { label: "vs Trigger.dev", href: "/compare/trigger-dev" },
    { label: "vs Hatchet", href: "/compare/hatchet" },
  ],
  Resources: [
    { label: "Blog", href: "/blog" },
    { label: "Pricing", href: "/pricing" },
    { label: "Documentation", href: "/docs" },
  ],
  Company: [
    { label: "Twitter", href: "https://twitter.com/leonardomso" },
    { label: "GitHub", href: "https://github.com/leonardomso/strait" },
    { label: "Privacy", href: "/privacy" },
    { label: "Terms", href: "/terms" },
  ],
};

const Footer = () => {
  const year = new Date().getFullYear();

  return (
    <footer className="border-border/40 border-t">
      <div className="mx-auto w-full max-w-[1600px] px-4 py-12 sm:px-6 sm:py-14 lg:px-8">
        <div className="flex flex-col justify-between gap-10 md:flex-row md:gap-16">
          <div className="flex shrink-0 flex-col items-start gap-3">
            <a
              className="inline-flex items-center transition-opacity hover:opacity-80"
              href="/"
            >
              <span className="sr-only">{siteConfig.name}</span>
              <img
                alt={siteConfig.logo.alt}
                className="h-8 w-auto"
                decoding="async"
                height={siteConfig.logo.height}
                loading="lazy"
                src={siteConfig.logo.src}
                width={siteConfig.logo.width}
              />
            </a>
            <p className="max-w-xs text-pretty text-muted-foreground/60 text-sm leading-relaxed">
              Open-source job orchestration for background jobs, workflows, and
              AI agents.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-8 sm:grid-cols-4">
            {Object.entries(FOOTER_LINKS).map(([group, links]) => (
              <div key={group}>
                <h3 className="font-semibold text-foreground text-sm">
                  {group}
                </h3>
                <ul className="mt-3 flex flex-col gap-2">
                  {links.map((link) => (
                    <li key={link.label}>
                      <a
                        className="text-muted-foreground/60 text-sm transition-colors hover:text-foreground"
                        href={link.href}
                        {...(link.href.startsWith("http")
                          ? { target: "_blank", rel: "noopener noreferrer" }
                          : {})}
                      >
                        {link.label}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>

        <div className="mt-10 border-border/40 border-t pt-5">
          <p className="text-muted-foreground/40 text-sm">
            &copy; {year} {siteConfig.name}. All rights reserved.
          </p>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
