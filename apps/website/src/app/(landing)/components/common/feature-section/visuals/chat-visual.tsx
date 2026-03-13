/* ------------------------------------------------------------------ */
/*  AI Chat Showcase — 4 animated mock-UI visuals                     */
/* ------------------------------------------------------------------ */

/** Visual 1 — Chat panel alongside document */
export const ChatVisual1 = () => (
  <div className="flex min-h-[280px] sm:min-h-[340px]">
    {/* Document side */}
    <div className="flex-1 border-border/40 border-r p-4">
      <div className="mb-3 flex items-center gap-2">
        <svg
          className="size-3.5 text-muted-foreground"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          viewBox="0 0 24 24"
        >
          <path
            d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
        <span className="font-medium text-foreground text-xs">Blog Draft</span>
      </div>
      <div className="space-y-2">
        <div className="h-2 w-full rounded bg-foreground/8" />
        <div className="h-2 w-11/12 rounded bg-foreground/8" />
        <div className="h-2 w-9/12 rounded bg-foreground/8" />
        <div className="mt-3 h-2 w-full rounded bg-foreground/8" />
        <div className="h-2 w-10/12 rounded bg-foreground/8" />
        <div className="h-2 w-8/12 rounded bg-foreground/8" />
        <div className="mt-3 h-2 w-full rounded bg-foreground/8" />
        <div className="h-2 w-7/12 rounded bg-foreground/8" />
      </div>
    </div>

    {/* Chat side */}
    <div className="flex w-2/5 flex-col bg-muted/10 sm:w-[45%]">
      <div className="border-border/40 border-b px-3 py-2">
        <span className="font-semibold text-foreground text-xs">AI Chat</span>
      </div>
      <div className="flex-1 space-y-2 overflow-hidden p-3">
        <div className="rounded-lg bg-muted px-2.5 py-1.5 text-foreground text-xs">
          Can you make paragraph 2 more engaging?
        </div>
        <div className="rounded-lg border border-border/40 bg-background px-2.5 py-1.5 text-foreground text-xs">
          I&apos;ve rewritten paragraph 2 with a stronger opening hook and more
          vivid language.
        </div>
        <div className="rounded-lg bg-muted px-2.5 py-1.5 text-foreground text-xs">
          Add some data points to support the claim.
        </div>
        <div className="rounded-lg border border-border/40 bg-background px-2.5 py-1.5 text-foreground text-xs">
          Added two statistics from recent studies
          <span className="ml-0.5 inline-block h-3 w-0.5 animate-pulse bg-primary" />
        </div>
      </div>
      <div className="border-border/40 border-t p-2">
        <div className="flex items-center gap-2 rounded-md border border-border/40 bg-background px-2 py-1.5">
          <span className="flex-1 text-muted-foreground text-xs">
            Ask anything about your draft...
          </span>
          <div className="flex size-5 items-center justify-center rounded bg-primary text-primary-foreground">
            <span className="text-xs">↑</span>
          </div>
        </div>
      </div>
    </div>
  </div>
);

/** Visual 2 — Text rewrite animation */
export const ChatVisual2 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Tone adjustment
    </p>

    {/* Before */}
    <div className="rounded-lg border border-border/40 bg-muted/20 p-3">
      <p className="mb-1 font-medium text-muted-foreground text-xs">Before</p>
      <p className="text-muted-foreground text-sm line-through decoration-red-400/50">
        The product has many features that are useful for businesses of varying
        sizes.
      </p>
    </div>

    {/* Arrow */}
    <div className="flex justify-center text-foreground">
      <svg
        className="size-5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        viewBox="0 0 24 24"
      >
        <path
          d="M12 5v14m0 0l-4-4m4 4l4-4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </div>

    {/* After */}
    <div className="rounded-lg border border-foreground/8 bg-muted/50 p-3">
      <p className="mb-1 font-medium text-foreground text-xs">After</p>
      <p className="text-foreground text-sm">
        Whether you&apos;re a startup or an enterprise,{" "}
        <span className="bg-muted font-medium text-foreground">
          every feature scales with your ambition
        </span>
        .
      </p>
    </div>
  </div>
);

/** Visual 3 — Paragraph restructuring */
export const ChatVisual3 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Restructure &amp; expand
    </p>

    <div className="space-y-2">
      {[
        { label: "Introduction", width: "w-full", active: false },
        { label: "Problem Statement", width: "w-11/12", active: true },
        { label: "Solution Overview", width: "w-10/12", active: false },
        { label: "Key Benefits", width: "w-full", active: false },
        { label: "Case Study", width: "w-9/12", active: true },
        { label: "Conclusion", width: "w-8/12", active: false },
      ].map((section) => (
        <div
          className={`flex items-center gap-3 rounded-md px-3 py-2 transition-colors ${
            section.active
              ? "border border-foreground/8 bg-muted/50"
              : "border border-border/40 bg-background"
          }`}
          key={section.label}
        >
          <div className="flex size-5 items-center justify-center">
            {section.active ? (
              <span className="text-foreground text-xs">↕</span>
            ) : (
              <span className="text-muted-foreground text-xs">≡</span>
            )}
          </div>
          <span
            className={`text-sm ${section.active ? "font-medium text-foreground" : "text-muted-foreground"}`}
          >
            {section.label}
          </span>
          <div
            className={`ml-auto h-1 ${section.width} max-w-[80px] rounded bg-foreground/10`}
          />
        </div>
      ))}
    </div>
  </div>
);

/** Visual 4 — Streaming text with cursor */
export const ChatVisual4 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Real-time streaming
    </p>

    <div className="rounded-lg border border-border/40 bg-background p-4">
      <div className="mb-3 flex items-center gap-2">
        <div className="flex size-6 items-center justify-center rounded-full bg-muted">
          <span className="font-semibold text-foreground text-xs">AI</span>
        </div>
        <span className="font-medium text-foreground text-xs">
          Writing assistant
        </span>
        <span className="ml-auto rounded-full bg-muted px-2 py-0.5 text-foreground text-xs">
          Streaming
        </span>
      </div>

      <div className="space-y-2">
        <p className="text-foreground text-sm leading-relaxed">
          Content marketing in 2025 requires a fundamentally different approach.
          The days of publishing generic blog posts and hoping for traffic are
          over.
        </p>
        <p className="text-foreground text-sm leading-relaxed">
          Instead, successful SaaS companies are investing in{" "}
          <span className="font-medium text-foreground">
            conversation-driven content
          </span>{" "}
          that speaks directly to
          <span className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-primary" />
        </p>
      </div>
    </div>

    <div className="flex items-center justify-between rounded-lg border border-border/40 bg-muted/20 px-3 py-1.5">
      <span className="text-muted-foreground text-xs">Generating...</span>
      <div className="flex gap-0.5">
        <span className="size-1 animate-bounce rounded-full bg-primary [animation-delay:0ms]" />
        <span className="size-1 animate-bounce rounded-full bg-primary [animation-delay:150ms]" />
        <span className="size-1 animate-bounce rounded-full bg-primary [animation-delay:300ms]" />
      </div>
    </div>
  </div>
);
