"use client";

import { useEffect, useState } from "react";

const LINES = [
  { prompt: true, text: "strait init" },
  { prompt: false, text: "Created strait.json" },
  { prompt: true, text: 'strait jobs create --name "process-upload"' },
  { prompt: false, text: "Job created: process-upload (id: job_7x2k9)" },
  {
    prompt: true,
    text: 'strait trigger process-upload --payload \'{"file": "report.pdf"}\'',
  },
  { prompt: false, text: "Run started: run_3m8p1 (status: queued)" },
  { prompt: false, text: "Run executing: run_3m8p1 -> POST https://..." },
  { prompt: false, text: "Run completed: run_3m8p1 (200 OK, 1.2s)" },
];

export function TerminalDemo() {
  const [visibleLines, setVisibleLines] = useState(0);

  useEffect(() => {
    if (visibleLines >= LINES.length) {
      return;
    }
    const delay = LINES[visibleLines]?.prompt ? 800 : 400;
    const timer = setTimeout(() => {
      setVisibleLines((prev) => prev + 1);
    }, delay);
    return () => clearTimeout(timer);
  }, [visibleLines]);

  return (
    <div className="mx-auto mt-12 w-full max-w-2xl overflow-hidden rounded-lg border border-border bg-[#0a0a0a]">
      <div className="flex items-center gap-2 border-border/50 border-b px-4 py-3">
        <div className="size-3 rounded-full bg-red-500/80" />
        <div className="size-3 rounded-full bg-yellow-500/80" />
        <div className="size-3 rounded-full bg-green-500/80" />
        <span className="ml-2 font-mono text-muted-foreground text-xs">
          terminal
        </span>
      </div>
      <div className="p-4 font-mono text-sm leading-relaxed">
        {LINES.slice(0, visibleLines).map((line) => (
          <div
            key={line.text}
            className={
              line.prompt ? "text-green-400" : "text-neutral-400"
            }
          >
            {line.prompt ? (
              <>
                <span className="text-blue-400">$</span> {line.text}
              </>
            ) : (
              <span className="ml-4">{line.text}</span>
            )}
          </div>
        ))}
        {visibleLines < LINES.length && (
          <span className="inline-block h-4 w-2 animate-pulse bg-green-400" />
        )}
      </div>
    </div>
  );
}
