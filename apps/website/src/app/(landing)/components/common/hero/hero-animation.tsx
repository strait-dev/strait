"use client";

import { useEffect, useRef } from "react";

type Line = {
  text: string;
  chars: number;
  dur: string;
  delay: string;
  end: string;
  last?: boolean;
};

const LINES: Line[] = [
  {
    text: "Tell me about your blog post idea.",
    chars: 38,
    dur: "1.2s",
    delay: "0.3s",
    end: "1.5s",
  },
  {
    text: "It's about AI writing workflows.",
    chars: 32,
    dur: "1s",
    delay: "1.8s",
    end: "2.8s",
  },
  {
    text: "Who's the target audience?",
    chars: 26,
    dur: "0.8s",
    delay: "3.1s",
    end: "3.9s",
  },
  {
    text: "Content creators & marketers.",
    chars: 29,
    dur: "0.9s",
    delay: "4.2s",
    end: "5.1s",
  },
  {
    text: "Great \u2014 generating 3 drafts\u2026",
    chars: 28,
    dur: "0.9s",
    delay: "5.4s",
    end: "6.3s",
    last: true,
  },
];

const DRAFTS = [
  {
    title: "Draft A — Practical Guide",
    desc: "Step-by-step workflow for teams adopting AI writing tools.",
  },
  {
    title: "Draft B — Thought Leadership",
    desc: "Why AI-assisted writing is the future of content creation.",
  },
  {
    title: "Draft C — Case Study",
    desc: "How a marketing team cut writing time by 60% with AI.",
  },
] as const;

const HeroAnimation = () => {
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
      { threshold: 0.2 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <section className="border-y bg-muted/30 py-12 sm:py-16 lg:py-20">
      <div
        className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8"
        ref={sectionRef}
      >
        <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[1fr_1.15fr] lg:gap-12">
          <div className="overflow-hidden rounded-xl border bg-card shadow-sm">
            <div className="flex items-center gap-1.5 border-b px-4 py-3">
              <span className="size-2.5 rounded-full bg-red-400/70" />
              <span className="size-2.5 rounded-full bg-yellow-400/70" />
              <span className="size-2.5 rounded-full bg-green-400/70" />
              <span className="ml-3 text-muted-foreground/50 text-xs">
                strait — interview
              </span>
            </div>

            <div className="space-y-1 p-5 sm:p-6">
              {LINES.map((line) => (
                <span
                  className={`hero-tl${line.last ? "hero-tl-last" : ""}`}
                  key={line.text}
                  style={
                    {
                      "--chars": line.chars,
                      "--dur": line.dur,
                      "--delay": line.delay,
                      "--end": line.end,
                    } as React.CSSProperties
                  }
                >
                  {line.text}
                </span>
              ))}
            </div>
          </div>

          <div className="flex flex-col gap-4">
            {DRAFTS.map((draft, idx) => (
              <div
                className="hero-draft-card paper-texture rounded-xl border bg-card p-5 shadow-sm transition-shadow hover:shadow-md sm:p-6"
                key={draft.title}
                style={
                  {
                    "--card-delay": `${6.6 + idx * 0.25}s`,
                  } as React.CSSProperties
                }
              >
                <p className="font-medium text-foreground text-sm">
                  {draft.title}
                </p>
                <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
                  {draft.desc}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};

export default HeroAnimation;
