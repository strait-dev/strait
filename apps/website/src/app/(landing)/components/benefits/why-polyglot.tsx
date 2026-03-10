"use client";

import { useEffect, useRef } from "react";

const ChatVisual = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div
      className="why-card-el flex justify-end"
      style={{ "--el-delay": "0.3s" } as React.CSSProperties}
    >
      <div className="rounded-2xl rounded-br-sm bg-primary-foreground/20 px-4 py-2.5">
        <p className="text-primary-foreground/90 text-sm">
          Why did this run move to dead letter?
        </p>
      </div>
    </div>

    <div
      className="why-card-el flex justify-start"
      style={{ "--el-delay": "0.9s" } as React.CSSProperties}
    >
      <div className="max-w-[85%] rounded-2xl rounded-bl-sm border border-primary-foreground/15 bg-primary-foreground/10 px-4 py-2.5">
        <p className="text-primary-foreground/80 text-sm leading-relaxed">
          Attempt 3 returned 502 and max attempts was reached. Replay is
          available with the same payload and metadata.
        </p>
      </div>
    </div>

    <div
      className="why-card-el flex justify-start"
      style={{ "--el-delay": "1.8s" } as React.CSSProperties}
    >
      <div className="flex gap-1.5 rounded-2xl rounded-bl-sm border border-primary-foreground/15 bg-primary-foreground/10 px-4 py-3">
        <span className="why-typing-dot size-1.5 rounded-full bg-primary-foreground/50" />
        <span
          className="why-typing-dot size-1.5 rounded-full bg-primary-foreground/50"
          style={{ animationDelay: "0.15s" }}
        />
        <span
          className="why-typing-dot size-1.5 rounded-full bg-primary-foreground/50"
          style={{ animationDelay: "0.3s" }}
        />
      </div>
    </div>
  </div>
);

const EditorVisual = () => (
  <div className="flex h-full flex-col px-6 py-8">
    <div
      className="why-card-el mb-4 flex items-center gap-2"
      style={{ "--el-delay": "0.2s" } as React.CSSProperties}
    >
      <div className="flex gap-1">
        <span className="size-2 rounded-full bg-primary-foreground/30" />
        <span className="size-2 rounded-full bg-primary-foreground/30" />
        <span className="size-2 rounded-full bg-primary-foreground/30" />
      </div>
      <span className="text-primary-foreground/40 text-xs">workflow.yaml</span>
    </div>

    <div
      className="why-card-el mb-3"
      style={{ "--el-delay": "0.4s" } as React.CSSProperties}
    >
      <div className="h-3.5 w-3/4 rounded bg-primary-foreground/25" />
    </div>

    <div className="space-y-2">
      <div
        className="why-card-el"
        style={{ "--el-delay": "0.6s" } as React.CSSProperties}
      >
        <div className="h-2.5 w-full rounded bg-primary-foreground/15" />
      </div>
      <div
        className="why-card-el"
        style={{ "--el-delay": "0.75s" } as React.CSSProperties}
      >
        <div className="h-2.5 w-[92%] rounded bg-primary-foreground/15" />
      </div>
      <div
        className="why-card-el"
        style={{ "--el-delay": "0.9s" } as React.CSSProperties}
      >
        <div className="h-2.5 w-[85%] rounded bg-primary-foreground/15" />
      </div>
    </div>

    <div
      className="why-card-el mt-4"
      style={{ "--el-delay": "1.2s" } as React.CSSProperties}
    >
      <div className="rounded-lg border border-primary-foreground/15 bg-primary-foreground/8 px-3 py-2.5">
        <div className="mb-1.5 h-2.5 w-[70%] rounded bg-primary-foreground/20" />
        <div className="h-2.5 w-[50%] rounded bg-primary-foreground/15" />
      </div>
    </div>

    <div
      className="why-card-el mt-3"
      style={{ "--el-delay": "1.5s" } as React.CSSProperties}
    >
      <span className="why-cursor inline-block h-4 w-0.5 bg-primary-foreground/60" />
    </div>
  </div>
);

const OrgVisual = () => (
  <div className="flex h-full flex-col px-6 py-8">
    <div
      className="why-card-el mb-4 flex items-center gap-2"
      style={{ "--el-delay": "0.2s" } as React.CSSProperties}
    >
      <div className="size-6 rounded-md bg-primary-foreground/20" />
      <span className="font-medium text-primary-foreground/70 text-xs">
        Platform Project
      </span>
    </div>

    {[
      { name: "Webhook Pipeline", count: 12, delay: "0.5s" },
      { name: "Billing Jobs", count: 8, delay: "0.7s" },
      { name: "Agent Workflow", count: 5, delay: "0.9s" },
    ].map((folder) => (
      <div
        className="why-card-el mb-2 flex items-center justify-between rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 px-3 py-2"
        key={folder.name}
        style={{ "--el-delay": folder.delay } as React.CSSProperties}
      >
        <div className="flex items-center gap-2">
          <div className="flex size-5 items-center justify-center rounded bg-primary-foreground/15">
            <svg
              className="size-3 text-primary-foreground/50"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              viewBox="0 0 24 24"
            >
              <path
                d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
          <span className="text-primary-foreground/70 text-xs">
            {folder.name}
          </span>
        </div>
        <span className="text-primary-foreground/40 text-xs">
          {folder.count}
        </span>
      </div>
    ))}

    <div
      className="why-card-el mt-3 flex flex-wrap gap-1.5"
      style={{ "--el-delay": "1.2s" } as React.CSSProperties}
    >
      {["Queued", "Executing", "Failed", "Completed"].map((tag) => (
        <span
          className="rounded-full border border-primary-foreground/15 bg-primary-foreground/10 px-2.5 py-0.5 text-primary-foreground/60 text-xs"
          key={tag}
        >
          {tag}
        </span>
      ))}
    </div>
  </div>
);

const CARDS = [
  {
    id: "runtime-feedback",
    title: "Run intelligence where work happens",
    description:
      "Inspect failures, retries, and terminal states with context-rich events tied to every execution.",
    Visual: ChatVisual,
  },
  {
    id: "workflow-control",
    title: "Configuration that stays operational",
    description:
      "Define jobs and workflow behavior in one place, then execute with predictable semantics at scale.",
    Visual: EditorVisual,
  },
  {
    id: "organization",
    title: "Projects organized for operations",
    description:
      "Environments, groups, and state labels keep asynchronous systems understandable as they grow.",
    Visual: OrgVisual,
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
      <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="font-bold text-foreground">
              Why engineering teams use Strait.
            </span>{" "}
            <span className="text-muted-foreground">
              Build reliable async systems with less platform overhead and
              better operational control.
            </span>
          </h2>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 lg:gap-8">
          {CARDS.map((card) => (
            <div className="flex flex-col" key={card.id}>
              <div className="why-card-visual relative aspect-square overflow-hidden rounded-2xl bg-primary">
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
      </div>
    </section>
  );
};

export default WhyStrait;
