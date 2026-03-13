/* ------------------------------------------------------------------ */
/*  Editor Showcase — 4 animated mock-UI visuals                      */
/* ------------------------------------------------------------------ */

/** Visual 1 — Editor toolbar with formatting buttons */
export const EditorVisual1 = () => {
  const toolbarItems = [
    { label: "B", bold: true },
    { label: "I", italic: true },
    { label: "U", underline: true },
    { label: "H1" },
    { label: "H2" },
    { label: "H3" },
    { label: "—", divider: true },
    { label: "•" },
    { label: "1." },
    { label: "☐" },
    { label: "—", divider: true },
    { label: "</>" },
    { label: "⌗" },
    { label: "▣" },
  ];

  return (
    <div className="flex flex-col p-6">
      <p className="mb-4 font-medium text-muted-foreground text-xs uppercase tracking-wider">
        Rich text editor
      </p>

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-1 rounded-t-lg border border-border/40 bg-muted/30 px-3 py-2">
        {toolbarItems.map((item, i) =>
          item.divider ? (
            <div
              className="mx-1 h-5 w-px bg-border/40"
              key={
                item.label === "—" && i === 6
                  ? "divider-after-h3"
                  : "divider-after-tasks"
              }
            />
          ) : (
            <button
              className="flex size-7 items-center justify-center rounded-md text-muted-foreground text-xs transition-colors hover:bg-background hover:text-foreground"
              key={item.label}
              style={{
                fontWeight: item.bold ? 700 : undefined,
                fontStyle: item.italic ? "italic" : undefined,
                textDecoration: item.underline ? "underline" : undefined,
              }}
              type="button"
            >
              {item.label}
            </button>
          )
        )}
      </div>

      {/* Editor content area */}
      <div className="rounded-b-lg border border-border/40 border-t-0 bg-background p-4">
        <h3 className="font-semibold text-base text-foreground">
          Scaling Content Marketing in 2025
        </h3>
        <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
          The landscape of content marketing has shifted dramatically.
          Here&apos;s what SaaS founders need to know about building a{" "}
          <span className="bg-muted text-foreground">
            sustainable content engine
          </span>{" "}
          that drives real growth.
        </p>
        <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
          In this post, we&apos;ll cover three strategies that...
        </p>
        <span className="inline-block h-4 w-0.5 animate-pulse bg-foreground" />
      </div>

      {/* Word count bar */}
      <div className="flex items-center justify-between rounded-b-lg border border-border/40 border-t-0 bg-muted/20 px-3 py-1.5">
        <span className="text-muted-foreground text-xs">842 words</span>
        <span className="text-muted-foreground text-xs">4 min read</span>
      </div>
    </div>
  );
};

/** Visual 2 — Folder tree with nested structure */
export const EditorVisual2 = () => (
  <div className="flex flex-col gap-3 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Workspaces &amp; folders
    </p>

    <div className="rounded-lg border border-border/40 bg-background p-3">
      {/* Workspace header */}
      <div className="flex items-center gap-2 pb-2">
        <div className="size-3 rounded-sm bg-primary" />
        <span className="font-semibold text-foreground text-sm">Marketing</span>
      </div>

      {/* Folder tree */}
      <div className="ml-2 space-y-1 border-border/40 border-l pl-3">
        {/* Expanded folder */}
        <div className="flex items-center gap-2 rounded-md bg-muted/50 px-2 py-1">
          <svg
            className="size-3.5 text-foreground"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            viewBox="0 0 24 24"
          >
            <path
              d="M3.75 9.776c.112-.017.227-.026.344-.026h15.812c.117 0 .232.009.344.026m-16.5 0a2.25 2.25 0 00-1.883 2.542l.857 6a2.25 2.25 0 002.227 1.932H19.05a2.25 2.25 0 002.227-1.932l.857-6a2.25 2.25 0 00-1.883-2.542m-16.5 0V6A2.25 2.25 0 016 3.75h3.879a1.5 1.5 0 011.06.44l2.122 2.12a1.5 1.5 0 001.06.44H18A2.25 2.25 0 0120.25 9v.776"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="font-medium text-foreground text-sm">
            Blog Posts
          </span>
        </div>
        <div className="ml-5 space-y-0.5 border-border/40 border-l pl-3">
          <div className="flex items-center gap-2 px-2 py-0.5">
            <svg
              className="size-3 text-muted-foreground"
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
            <span className="text-muted-foreground text-xs">
              Scaling Content.md
            </span>
          </div>
          <div className="flex items-center gap-2 px-2 py-0.5">
            <svg
              className="size-3 text-muted-foreground"
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
            <span className="text-muted-foreground text-xs">
              AI Writing Tools.md
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2 px-2 py-1">
          <svg
            className="size-3.5 text-muted-foreground"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            viewBox="0 0 24 24"
          >
            <path
              d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="text-muted-foreground text-sm">Social Media</span>
        </div>
        <div className="flex items-center gap-2 px-2 py-1">
          <svg
            className="size-3.5 text-muted-foreground"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            viewBox="0 0 24 24"
          >
            <path
              d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="text-muted-foreground text-sm">Email Campaigns</span>
        </div>
      </div>
    </div>

    {/* Second workspace */}
    <div className="rounded-lg border border-border/40 bg-background p-3 opacity-60">
      <div className="flex items-center gap-2">
        <div className="size-3 rounded-sm bg-foreground/20" />
        <span className="font-medium text-foreground text-sm">
          Product Docs
        </span>
      </div>
    </div>
  </div>
);

/** Visual 3 — Tag pills on a document card */
export const EditorVisual3 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Custom tags
    </p>

    <div className="rounded-lg border border-border/40 bg-background p-4">
      <div className="flex items-start justify-between">
        <div>
          <h4 className="font-semibold text-foreground text-sm">
            Scaling Content Marketing
          </h4>
          <p className="mt-0.5 text-muted-foreground text-xs">
            Updated 2 hours ago · 842 words
          </p>
        </div>
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
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        <span
          className="animate-fade-in-up rounded-full bg-muted px-2.5 py-0.5 font-medium text-foreground text-xs"
          style={{ animationDelay: "0ms", animationFillMode: "both" }}
        >
          Marketing
        </span>
        <span
          className="animate-fade-in-up rounded-full bg-muted/60 px-2.5 py-0.5 font-medium text-primary/80 text-xs"
          style={{ animationDelay: "100ms", animationFillMode: "both" }}
        >
          Blog
        </span>
        <span
          className="animate-fade-in-up rounded-full bg-primary/6 px-2.5 py-0.5 font-medium text-primary/70 text-xs"
          style={{ animationDelay: "200ms", animationFillMode: "both" }}
        >
          Q1 2025
        </span>
      </div>
    </div>

    {/* Tag management preview */}
    <div className="rounded-lg border border-border/40 bg-muted/20 p-3">
      <p className="mb-2 text-muted-foreground text-xs">Popular tags</p>
      <div className="flex flex-wrap gap-1">
        {[
          "Marketing",
          "Blog",
          "Newsletter",
          "Draft",
          "Published",
          "Q1 2025",
        ].map((tag) => (
          <span
            className="rounded-full border border-border/40 bg-background px-2 py-0.5 text-muted-foreground text-xs"
            key={tag}
          >
            {tag}
          </span>
        ))}
      </div>
    </div>
  </div>
);

/** Visual 4 — Export dropdown */
export const EditorVisual4 = () => {
  const formats = [
    {
      name: "PDF Document",
      ext: ".pdf",
      abbr: "PDF",
      desc: "Print-ready format",
    },
    {
      name: "Word Document",
      ext: ".docx",
      abbr: "DOC",
      desc: "Microsoft Word",
    },
    {
      name: "Markdown",
      ext: ".md",
      abbr: "MD",
      desc: "Plain text with formatting",
    },
    { name: "Plain Text", ext: ".txt", abbr: "TXT", desc: "No formatting" },
  ];

  return (
    <div className="flex flex-col gap-4 p-6">
      <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
        Export your work
      </p>

      <div className="rounded-lg border border-border/40 bg-background p-1">
        {formats.map((fmt, i) => (
          <button
            className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors ${
              i === 0 ? "bg-muted/50" : "hover:bg-muted/50"
            }`}
            key={fmt.ext}
            type="button"
          >
            <span className="flex size-8 items-center justify-center rounded-md bg-muted font-bold text-foreground text-xs">
              {fmt.abbr}
            </span>
            <div className="flex-1">
              <p className="font-medium text-foreground text-sm">{fmt.name}</p>
              <p className="text-muted-foreground text-xs">{fmt.desc}</p>
            </div>
            <span className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs">
              {fmt.ext}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
};
