/* ------------------------------------------------------------------ */
/*  Documents Showcase — 4 animated mock-UI visuals                   */
/* ------------------------------------------------------------------ */

/** Visual 1 — Document list cards */
export const DocumentsVisual1 = () => {
  const docs = [
    {
      title: "Scaling Content Marketing",
      words: "842 words",
      time: "2h ago",
      pinned: true,
    },
    {
      title: "Product Launch Playbook",
      words: "1,230 words",
      time: "5h ago",
      pinned: false,
    },
    {
      title: "Q1 Newsletter Draft",
      words: "456 words",
      time: "1d ago",
      pinned: false,
    },
    {
      title: "Twitter Thread: AI Tools",
      words: "380 words",
      time: "2d ago",
      pinned: false,
    },
  ];

  return (
    <div className="flex flex-col gap-3 p-6">
      <div className="flex items-center justify-between">
        <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
          Recent documents
        </p>
        <span className="text-muted-foreground text-xs">4 documents</span>
      </div>

      <div className="space-y-1.5">
        {docs.map((doc) => (
          <div
            className="flex items-center gap-3 rounded-lg border border-border/40 bg-background px-3 py-2.5 transition-colors hover:bg-muted/30"
            key={doc.title}
          >
            {doc.pinned ? (
              <svg
                className="size-4 text-foreground"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                viewBox="0 0 24 24"
              >
                <path
                  d="M16.5 3.75V16.5L12 14.25 7.5 16.5V3.75h9z"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            ) : (
              <svg
                className="size-4 text-muted-foreground"
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
            )}
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium text-foreground text-sm">
                {doc.title}
              </p>
              <p className="text-muted-foreground text-xs">
                {doc.words} · {doc.time}
              </p>
            </div>
            <span className="text-muted-foreground text-xs">→</span>
          </div>
        ))}
      </div>
    </div>
  );
};

/** Visual 2 — Auto-save indicator */
export const DocumentsVisual2 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Auto-save
    </p>

    {/* Editor mock */}
    <div className="rounded-lg border border-border/40 bg-background">
      <div className="flex items-center justify-between border-border/40 border-b px-3 py-2">
        <span className="font-medium text-foreground text-xs">
          Scaling Content Marketing
        </span>
        <div className="flex items-center gap-1.5">
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-foreground/20 opacity-75" />
            <span className="relative inline-flex size-2 rounded-full bg-primary" />
          </span>
          <span className="text-foreground text-xs">Saved</span>
        </div>
      </div>
      <div className="space-y-2 p-4">
        <div className="h-2 w-full rounded bg-foreground/8" />
        <div className="h-2 w-10/12 rounded bg-foreground/8" />
        <div className="h-2 w-8/12 rounded bg-foreground/8" />
      </div>
    </div>

    {/* Save history */}
    <div className="space-y-1.5">
      {[
        { time: "Just now", status: "Auto-saved" },
        { time: "2 min ago", status: "Auto-saved" },
        { time: "5 min ago", status: "Auto-saved" },
      ].map((entry) => (
        <div
          className="flex items-center justify-between text-xs"
          key={entry.time}
        >
          <span className="text-muted-foreground">{entry.time}</span>
          <span className="flex items-center gap-1 text-foreground">
            <svg
              className="size-3"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              viewBox="0 0 24 24"
            >
              <path
                d="M5 13l4 4L19 7"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            {entry.status}
          </span>
        </div>
      ))}
    </div>
  </div>
);

/** Visual 3 — Multi-device sync */
export const DocumentsVisual3 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Real-time sync
    </p>

    <div className="flex items-center justify-center gap-6">
      {/* Desktop device */}
      <div className="flex flex-col items-center gap-1.5">
        <div className="w-28 rounded-lg border border-border/40 bg-background p-2">
          <div className="rounded bg-muted/30 p-1.5">
            <div className="space-y-1">
              <div className="h-1 w-full rounded bg-foreground/10" />
              <div className="h-1 w-9/12 rounded bg-foreground/10" />
              <div className="h-1 w-11/12 rounded bg-foreground/10" />
            </div>
          </div>
        </div>
        <span className="text-muted-foreground text-xs">Desktop</span>
      </div>

      {/* Sync indicator */}
      <div className="flex flex-col items-center gap-1">
        <div className="flex items-center gap-1">
          <span className="h-px w-4 bg-primary/40" />
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-50" />
            <span className="relative inline-flex size-2 rounded-full bg-primary" />
          </span>
          <span className="h-px w-4 bg-primary/40" />
        </div>
        <span className="font-medium text-foreground text-xs">Synced</span>
      </div>

      {/* Mobile device */}
      <div className="flex flex-col items-center gap-1.5">
        <div className="w-16 rounded-lg border border-border/40 bg-background p-1.5">
          <div className="rounded bg-muted/30 p-1">
            <div className="space-y-0.5">
              <div className="h-0.5 w-full rounded bg-foreground/10" />
              <div className="h-0.5 w-9/12 rounded bg-foreground/10" />
              <div className="h-0.5 w-11/12 rounded bg-foreground/10" />
            </div>
          </div>
        </div>
        <span className="text-muted-foreground text-xs">Mobile</span>
      </div>
    </div>

    <p className="text-center text-muted-foreground text-xs">
      Changes appear everywhere in real time
    </p>
  </div>
);

/** Visual 4 — Search with filtered results */
export const DocumentsVisual4 = () => (
  <div className="flex flex-col gap-3 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Quick search
    </p>

    {/* Search input */}
    <div className="flex items-center gap-2 rounded-lg border border-foreground/10 bg-background px-3 py-2 shadow-sm">
      <svg
        className="size-4 text-foreground"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        viewBox="0 0 24 24"
      >
        <path
          d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <span className="text-foreground text-sm">content marketing</span>
    </div>

    {/* Results */}
    <div className="space-y-1">
      <p className="text-muted-foreground text-xs">3 results found</p>
      {[
        {
          title: "Scaling Content Marketing",
          match: "...sustainable content engine that drives...",
        },
        {
          title: "Q1 Content Strategy",
          match: "...content marketing budget allocation...",
        },
        {
          title: "Competitor Analysis",
          match: "...their content marketing approach differs...",
        },
      ].map((result) => (
        <div
          className="rounded-lg border border-border/40 bg-background p-2.5 transition-colors hover:bg-muted/30"
          key={result.title}
        >
          <p className="font-medium text-foreground text-sm">{result.title}</p>
          <p className="mt-0.5 text-muted-foreground text-xs">
            ...
            {result.match.split("content").map((part, i, arr) =>
              i < arr.length - 1 ? (
                <span key={`${result.title}-${part}`}>
                  {part}
                  <mark className="bg-muted text-foreground">content</mark>
                </span>
              ) : (
                <span key={`${result.title}-${part}`}>{part}</span>
              )
            )}
          </p>
        </div>
      ))}
    </div>
  </div>
);
