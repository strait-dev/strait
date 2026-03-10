"use client";

import { useEffect, useRef } from "react";

const EDITOR_LINES = [
  { text: "Order Processing Workflow", style: "heading" as const },
  { text: "", style: "spacer" as const },
  {
    text: "Step 1 validates incoming payload and checks idempotency before enqueueing work for downstream services.",
    style: "paragraph" as const,
  },
  { text: "", style: "spacer" as const },
  {
    text: "Queueing and state transitions stay in PostgreSQL, so workers scale horizontally without duplicate claims.",
    style: "highlight" as const,
  },
  { text: "", style: "spacer" as const },
  {
    text: "If the dispatch fails, Strait schedules retries with jitter and routes exhausted attempts to dead letter for replay.",
    style: "paragraph" as const,
  },
] as const;

const AI_MESSAGES = [
  {
    role: "user" as const,
    text: "Run is failing on attempt 2. What should we check?",
  },
  {
    role: "assistant" as const,
    text: "Dispatch returned 503 from the endpoint. Retry strategy is exponential with jitter. Next retry is scheduled for 12:04:18Z.",
  },
  {
    role: "user" as const,
    text: "Can we inspect full execution details?",
  },
  {
    role: "assistant" as const,
    text: "Yes. Open the debug bundle for this run to review events, checkpoints, usage, and tool-call outputs before replaying.",
  },
];

const InteractiveDemo = () => {
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
      { threshold: 0.15 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <section className="py-20 sm:py-28">
      <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="font-bold text-foreground">
              See how runs move through your system in real time.
            </span>{" "}
            <span className="text-muted-foreground">
              Track execution details, inspect failures, and decide the next
              action without leaving your operational workflow.
            </span>
          </h2>
        </div>

        {/* Primary background wrapper — matches feature showcase style */}
        <div className="min-h-[800px] rounded-2xl bg-primary/20 p-3 sm:p-4">
          <div
            className="paper-texture flex min-h-[calc(800px-2rem)] flex-col overflow-hidden rounded-xl border border-primary/15 bg-card shadow-lg"
            ref={sectionRef}
          >
            {/* Window chrome */}
            <div className="flex items-center justify-between border-border/50 border-b px-4 py-3">
              <div className="flex items-center gap-1.5">
                <span className="size-3 rounded-full bg-border" />
                <span className="size-3 rounded-full bg-border" />
                <span className="size-3 rounded-full bg-border" />
              </div>
              <span className="text-muted-foreground/50 text-xs">
                strait — run diagnostics
              </span>
              <div className="w-12" />
            </div>

            <div className="grid flex-1 grid-cols-1 lg:grid-cols-[1fr_380px]">
              <div className="border-border/50 p-8 sm:p-10 lg:border-r lg:p-12">
                <div className="space-y-0">
                  {EDITOR_LINES.map((line, idx) => {
                    if (line.style === "spacer") {
                      return (
                        <div className="h-4" key={`spacer-${String(idx)}`} />
                      );
                    }

                    if (line.style === "heading") {
                      return (
                        <div
                          className="demo-line"
                          key={line.text}
                          style={
                            {
                              "--line-delay": `${0.3 + idx * 0.15}s`,
                            } as React.CSSProperties
                          }
                        >
                          <h3 className="font-bold text-2xl text-foreground sm:text-3xl">
                            {line.text}
                          </h3>
                        </div>
                      );
                    }

                    if (line.style === "highlight") {
                      return (
                        <div
                          className="demo-line"
                          key={line.text}
                          style={
                            {
                              "--line-delay": `${0.3 + idx * 0.15}s`,
                            } as React.CSSProperties
                          }
                        >
                          <p className="border-primary/30 border-l-2 pl-4 text-foreground text-lg italic leading-relaxed">
                            {line.text}
                          </p>
                        </div>
                      );
                    }

                    return (
                      <div
                        className="demo-line"
                        key={line.text}
                        style={
                          {
                            "--line-delay": `${0.3 + idx * 0.15}s`,
                          } as React.CSSProperties
                        }
                      >
                        <p className="text-muted-foreground leading-relaxed">
                          {line.text}
                        </p>
                      </div>
                    );
                  })}

                  <div
                    className="demo-line mt-6"
                    style={{ "--line-delay": "1.6s" } as React.CSSProperties}
                  >
                    <span className="demo-cursor inline-block h-5 w-0.5 bg-primary" />
                  </div>
                </div>
              </div>

              <div className="flex flex-col border-border/50 border-t bg-muted/30 lg:border-t-0">
                <div className="border-border/50 border-b px-4 py-3">
                  <div className="flex items-center gap-2">
                    <div className="size-2 rounded-full bg-primary/60" />
                    <span className="font-medium text-foreground text-xs">
                      AI Assistant
                    </span>
                  </div>
                </div>

                <div className="flex-1 space-y-4 p-5 sm:p-6">
                  {AI_MESSAGES.map((msg, idx) => (
                    <div
                      className="demo-chat-msg"
                      key={`msg-${String(idx)}`}
                      style={
                        {
                          "--msg-delay": `${2.0 + idx * 0.8}s`,
                        } as React.CSSProperties
                      }
                    >
                      {msg.role === "user" ? (
                        <div className="flex justify-end">
                          <div className="max-w-[85%] rounded-xl rounded-br-sm bg-primary/10 px-3 py-2">
                            <p className="text-foreground text-sm">
                              {msg.text}
                            </p>
                          </div>
                        </div>
                      ) : (
                        <div className="flex justify-start">
                          <div className="max-w-[85%] rounded-xl rounded-bl-sm border border-border/50 bg-card px-3 py-2">
                            <p className="text-muted-foreground text-sm leading-relaxed">
                              {msg.text}
                            </p>
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>

                <div className="border-border/50 border-t px-4 py-3">
                  <div className="flex items-center rounded-lg border border-border/50 bg-card px-3 py-2">
                    <span className="text-muted-foreground/40 text-sm">
                      Ask about this run...
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-medium text-foreground text-sm">
              Live run timeline
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              See where each run is right now without jumping between tools.
            </p>
          </div>
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-medium text-foreground text-sm">Smart retries</p>
            <p className="mt-1 text-muted-foreground text-sm">
              Failed steps retry automatically so your team spends less time
              firefighting.
            </p>
          </div>
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-medium text-foreground text-sm">Fast replay</p>
            <p className="mt-1 text-muted-foreground text-sm">
              Replay failed runs in seconds and keep delivery moving.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
};

export default InteractiveDemo;
