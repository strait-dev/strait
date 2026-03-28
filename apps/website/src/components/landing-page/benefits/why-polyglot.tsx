import { useEffect, useRef, useState } from "react";
import Shell from "@/components/layout/shell.tsx";

const StackCollapseVisual = () => {
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => {
      setCollapsed((prev) => !prev);
    }, 3000);
    return () => clearInterval(interval);
  }, []);

  const brokers = ["RabbitMQ", "SQS", "Redis"];

  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-6 py-8">
      {brokers.map((label, i) => {
        const isHidden = collapsed;
        return (
          <div
            className="flex w-40 items-center justify-center rounded-lg border border-primary-foreground/20 bg-primary-foreground/10 px-4 py-2 text-primary-foreground/70 text-sm transition-all duration-500"
            key={label}
            style={{
              opacity: isHidden ? 0 : 1,
              transform: isHidden
                ? "translateY(20px) scale(0.9)"
                : "translateY(0) scale(1)",
              transitionDelay: `${i * 80}ms`,
            }}
          >
            {label}
          </div>
        );
      })}
      <div
        className="flex w-40 items-center justify-center rounded-lg border border-primary-foreground/40 bg-primary-foreground/20 px-4 py-2.5 font-medium text-primary-foreground/90 text-sm transition-all duration-500"
        style={{
          transform: collapsed ? "scale(1.08)" : "scale(1)",
          boxShadow: collapsed ? "0 0 20px oklch(1 0 0 / 0.1)" : "none",
        }}
      >
        Postgres
      </div>
    </div>
  );
};

const FSMVisual = () => {
  const states = ["queued", "claimed", "executing", "completed"];

  return (
    <div className="flex h-full flex-col items-center justify-center px-6 py-8">
      <div className="flex flex-col gap-0">
        {states.map((state, i) => (
          <div className="flex flex-col items-center" key={state}>
            <div
              className="why-card-el relative flex items-center gap-2"
              style={
                { "--el-delay": `${0.2 + i * 0.25}s` } as React.CSSProperties
              }
            >
              <div className="relative flex items-center justify-center rounded-md border border-primary-foreground/20 bg-primary-foreground/10 px-3 py-1.5">
                <span className="text-primary-foreground/80 text-xs">
                  {state}
                </span>
                <span
                  className="fsm-dot absolute -left-1.5 size-2 rounded-full bg-primary-foreground/80"
                  style={{ animationDelay: `${i * 0.8}s` }}
                />
              </div>
            </div>
            {i < states.length - 1 && (
              <div className="h-4 w-px bg-primary-foreground/20" />
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

const ObservabilityVisual = () => {
  const layers = ["Health", "Usage", "Debug Bundle", "Events"];

  return (
    <div className="flex h-full items-center justify-center px-6 py-8">
      <div className="relative h-32 w-48">
        {layers.map((label, i) => (
          <div
            className="why-card-el absolute top-0 left-0 w-full rounded-lg border border-primary-foreground/15 bg-primary-foreground/10 px-4 py-3 shadow-sm"
            key={label}
            style={
              {
                "--el-delay": `${0.3 + i * 0.2}s`,
                transform: `translateY(${i * 14}px) translateX(${i * 6}px) rotate(${i * -2}deg)`,
                zIndex: layers.length - i,
              } as React.CSSProperties
            }
          >
            <span className="font-medium text-primary-foreground/80 text-xs">
              {label}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

const AIWorkflowVisual = () => (
  <div className="flex h-full flex-col items-center justify-center gap-4 px-6 py-8">
    <div
      className="why-card-el flex items-center gap-3 rounded-lg border border-primary-foreground/20 bg-primary-foreground/10 px-4 py-3"
      style={{ "--el-delay": "0.3s" } as React.CSSProperties}
    >
      <span className="text-primary-foreground/50 text-xs">Tokens</span>
      <span className="font-medium font-mono text-primary-foreground/90 text-sm">
        12,847
      </span>
      <div className="h-4 w-px bg-primary-foreground/20" />
      <span className="text-primary-foreground/50 text-xs">Cost</span>
      <span className="font-medium font-mono text-primary-foreground/90 text-sm">
        $0.38
      </span>
    </div>

    <div
      className="why-card-el flex items-center gap-2 rounded-full border border-primary-foreground/25 bg-primary-foreground/15 px-3 py-1.5"
      style={{ "--el-delay": "0.8s" } as React.CSSProperties}
    >
      <span className="size-2 rounded-full bg-green-400/80" />
      <span className="font-medium text-primary-foreground/80 text-xs">
        Approved
      </span>
    </div>
  </div>
);

const CARDS = [
  {
    id: "zero-broker",
    title: "Zero brokers to manage",
    description:
      "Postgres is your queue. No Redis cluster, no RabbitMQ — one fewer service to provision, monitor, and pay for.",
    Visual: StackCollapseVisual,
  },
  {
    id: "fsm-states",
    title: "No ambiguous job states",
    description:
      "Every transition from queued to terminal is tracked and queryable. Know exactly where every run is, always.",
    Visual: FSMVisual,
  },
  {
    id: "observability",
    title: "Observability out of the box",
    description:
      "Events, debug bundles, usage tracking, and health scoring — included, not bolted on after your first outage.",
    Visual: ObservabilityVisual,
  },
  {
    id: "ai-workflows",
    title: "Cost controls built in",
    description:
      "Set per-run and daily budgets, track token spend, and require human approval before expensive steps execute.",
    Visual: AIWorkflowVisual,
  },
] as const;

const WhyStrait = () => {
  const sectionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          el.setAttribute("data-visible", "");
          observer.disconnect();
        }
      },
      { threshold: 0.1 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <section className="py-20 sm:py-28" ref={sectionRef}>
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">Less infra. More shipping.</span>{" "}
            <span className="text-muted-foreground">
              Replace your patchwork of queues, cron, and retry wrappers with
              one runtime your whole team can reason about.
            </span>
          </h2>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8">
          {CARDS.map((card) => (
            <div className="flex flex-col" key={card.id}>
              <div className="why-card-visual relative aspect-[4/3] overflow-hidden rounded-2xl bg-primary">
                <div className="showcase-dots pointer-events-none absolute inset-0" />
                <div
                  className="pointer-events-none absolute inset-0 opacity-30"
                  style={{
                    background:
                      "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)",
                  }}
                />
                <div className="relative z-10 h-full">
                  <card.Visual />
                </div>
              </div>
              <div className="mt-5">
                <h3 className="font-semibold text-foreground text-lg">
                  {card.title}
                </h3>
                <p className="mt-2 text-base text-muted-foreground leading-relaxed">
                  {card.description}
                </p>
              </div>
            </div>
          ))}
        </div>
      </Shell>
    </section>
  );
};

export default WhyStrait;
