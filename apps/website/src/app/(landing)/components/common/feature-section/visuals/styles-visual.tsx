/* ------------------------------------------------------------------ */
/*  Styles Showcase — 4 animated mock-UI visuals                      */
/* ------------------------------------------------------------------ */

/** Visual 1 — Formality & Energy sliders */
export const StylesVisual1 = () => (
  <div className="flex flex-col gap-6 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Define your voice
    </p>

    {/* Formality slider */}
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="font-medium text-foreground text-sm">Formality</span>
        <span className="rounded-md bg-muted px-2 py-0.5 font-semibold text-foreground text-xs">
          3/5
        </span>
      </div>
      <div className="relative h-2 w-full rounded-full bg-muted">
        <div
          className="absolute inset-y-0 left-0 rounded-full bg-primary transition-all duration-1000"
          style={{ width: "60%" }}
        />
        <div
          className="absolute top-1/2 size-4 -translate-y-1/2 rounded-full border-2 border-primary bg-background shadow-sm transition-all duration-1000"
          style={{ left: "calc(60% - 8px)" }}
        />
      </div>
      <div className="flex justify-between text-muted-foreground text-xs">
        <span>Casual</span>
        <span>Formal</span>
      </div>
    </div>

    {/* Energy slider */}
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="font-medium text-foreground text-sm">Energy</span>
        <span className="rounded-md bg-muted px-2 py-0.5 font-semibold text-foreground text-xs">
          4/5
        </span>
      </div>
      <div className="relative h-2 w-full rounded-full bg-muted">
        <div
          className="absolute inset-y-0 left-0 rounded-full bg-primary/70 transition-all duration-1000"
          style={{ width: "80%" }}
        />
        <div
          className="absolute top-1/2 size-4 -translate-y-1/2 rounded-full border-2 border-primary/70 bg-background shadow-sm transition-all duration-1000"
          style={{ left: "calc(80% - 8px)" }}
        />
      </div>
      <div className="flex justify-between text-muted-foreground text-xs">
        <span>Calm</span>
        <span>Energetic</span>
      </div>
    </div>

    {/* Preview */}
    <div className="rounded-lg border border-border/40 bg-muted/20 p-3">
      <p className="text-muted-foreground text-xs">Preview tone:</p>
      <p className="mt-1 text-foreground text-sm italic">
        &quot;Let&apos;s talk about a strategy that actually works — no fluff,
        just results.&quot;
      </p>
    </div>
  </div>
);

/** Visual 2 — File upload with analysis progress */
export const StylesVisual2 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Upload writing samples
    </p>

    {/* Drop zone */}
    <div className="flex flex-col items-center gap-2 rounded-lg border-2 border-border/50 border-dashed bg-muted/10 p-6">
      <div className="rounded-full bg-muted p-2">
        <svg
          className="size-5 text-foreground"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          viewBox="0 0 24 24"
        >
          <path
            d="M12 16V4m0 0L8 8m4-4l4 4M4 20h16"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </div>
      <p className="text-muted-foreground text-sm">
        Drop PDF, TXT, or Markdown
      </p>
    </div>

    {/* Uploaded file with progress */}
    <div className="rounded-lg border border-border/40 bg-background p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="rounded-md bg-muted p-1.5">
            <span className="text-foreground text-xs">PDF</span>
          </div>
          <div>
            <p className="font-medium text-foreground text-sm">
              quarterly-report.pdf
            </p>
            <p className="text-muted-foreground text-xs">128 KB</p>
          </div>
        </div>
        <span className="font-medium text-foreground text-xs">
          Analyzing...
        </span>
      </div>
      <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div
          className="h-full animate-pulse rounded-full bg-primary/70 transition-all duration-700"
          style={{ width: "72%" }}
        />
      </div>
    </div>
  </div>
);

/** Visual 3 — URL extraction */
export const StylesVisual3 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Extract style from URL
    </p>

    {/* URL input */}
    <div className="flex items-center gap-2 rounded-lg border border-border/40 bg-background px-3 py-2">
      <svg
        className="size-4 text-muted-foreground"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        viewBox="0 0 24 24"
      >
        <path
          d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M10.172 13.828a4 4 0 005.656 0l4-4a4 4 0 10-5.656-5.656l-1.102 1.101"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <span className="flex-1 text-foreground text-sm">
        https://example.com/blog/scaling-tips
      </span>
      <span className="rounded-md bg-muted px-2 py-0.5 font-medium text-foreground text-xs">
        ✓ Done
      </span>
    </div>

    {/* Extracted preview */}
    <div className="rounded-lg border border-border/40 bg-muted/20 p-4">
      <p className="font-medium text-muted-foreground text-xs">
        Extracted text preview
      </p>
      <div className="mt-2 space-y-1">
        <div className="h-2.5 w-full rounded bg-foreground/10" />
        <div className="h-2.5 w-11/12 rounded bg-foreground/10" />
        <div className="h-2.5 w-9/12 rounded bg-foreground/10" />
        <div className="h-2.5 w-10/12 rounded bg-foreground/10" />
      </div>
      <p className="mt-3 text-muted-foreground text-xs">
        1,240 words extracted · Ready for analysis
      </p>
    </div>
  </div>
);

/** Visual 4 — Style profile bound to session */
export const StylesVisual4 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Apply profile to session
    </p>

    {/* Style profile card */}
    <div className="rounded-lg border border-foreground/8 bg-muted/50 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="size-3 rounded-full bg-primary" />
          <span className="font-semibold text-foreground text-sm">
            Professional Voice
          </span>
        </div>
        <span className="rounded-full bg-muted px-2 py-0.5 font-medium text-foreground text-xs">
          Active
        </span>
      </div>
      <div className="mt-2 flex gap-3 text-muted-foreground text-xs">
        <span>Formality: 4/5</span>
        <span>·</span>
        <span>Energy: 3/5</span>
      </div>
    </div>

    {/* Arrow indicator */}
    <div className="flex justify-center text-muted-foreground/40">
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

    {/* Session card */}
    <div className="rounded-lg border border-border/40 bg-background p-4">
      <div className="flex items-center justify-between">
        <span className="font-medium text-foreground text-sm">
          Blog Post: Scaling Content
        </span>
        <span className="rounded-md bg-muted px-2 py-0.5 text-foreground text-xs">
          Writing
        </span>
      </div>
      <p className="mt-1 text-muted-foreground text-xs">
        Using &quot;Professional Voice&quot; profile
      </p>
    </div>
  </div>
);
