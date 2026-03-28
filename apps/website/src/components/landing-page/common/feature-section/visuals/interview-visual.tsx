/* ------------------------------------------------------------------ */
/*  Interview Showcase — 4 animated mock-UI visuals                   */
/* ------------------------------------------------------------------ */

/** Visual 1 — AI interviewer chat bubble with typing dots */
export const InterviewVisual1 = () => (
  <div className="flex flex-col gap-4 p-6">
    {/* AI message */}
    <div className="flex items-start gap-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted font-semibold text-foreground text-xs">
        AI
      </div>
      <div className="space-y-2">
        <div className="rounded-lg rounded-tl-none border border-border/40 bg-muted/30 px-4 py-2.5">
          <p className="text-foreground text-sm">
            What&apos;s the purpose of your content? Are you trying to inform,
            persuade, or entertain?
          </p>
        </div>
        <div className="rounded-lg rounded-tl-none border border-border/40 bg-muted/30 px-4 py-2.5">
          <p className="text-foreground text-sm">
            Who is your target audience?
          </p>
        </div>
      </div>
    </div>

    {/* User reply */}
    <div className="flex items-start justify-end gap-3">
      <div className="rounded-lg rounded-tr-none bg-muted px-4 py-2.5">
        <p className="text-foreground text-sm">
          I&apos;m writing a blog post for SaaS founders about scaling content
          marketing...
        </p>
      </div>
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-foreground/10 font-semibold text-foreground text-xs">
        You
      </div>
    </div>

    {/* AI typing indicator */}
    <div className="flex items-start gap-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted font-semibold text-foreground text-xs">
        AI
      </div>
      <div className="rounded-lg rounded-tl-none border border-border/40 bg-muted/30 px-4 py-3">
        <div className="flex items-center gap-1">
          <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:0ms]" />
          <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:150ms]" />
          <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:300ms]" />
        </div>
      </div>
    </div>
  </div>
);

/** Visual 2 — Three draft angle cards */
export const InterviewVisual2 = () => {
  const angles = [
    {
      name: "Hook-First",
      desc: "Open with a bold stat that grabs attention immediately.",
      color: "bg-muted text-foreground",
    },
    {
      name: "Story-Led",
      desc: "Start with a founder's journey to illustrate the problem.",
      color: "bg-muted/60 text-primary/80",
    },
    {
      name: "Data-Driven",
      desc: "Lead with research and benchmarks to build credibility.",
      color: "bg-primary/6 text-primary/70",
    },
  ];

  return (
    <div className="flex flex-col gap-3 p-6">
      <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
        3 draft angles generated
      </p>
      {angles.map((angle, i) => (
        <div
          className="animate-fade-in-up rounded-lg border border-border/40 bg-background p-4 transition-shadow hover:shadow-sm"
          key={angle.name}
          style={{ animationDelay: `${i * 120}ms`, animationFillMode: "both" }}
        >
          <div className="flex items-center gap-2">
            <span
              className={`rounded-md px-2 py-0.5 font-semibold text-xs ${angle.color}`}
            >
              {angle.name}
            </span>
          </div>
          <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
            {angle.desc}
          </p>
        </div>
      ))}
    </div>
  );
};

/** Visual 3 — Chat refinement with streaming cursor */
export const InterviewVisual3 = () => (
  <div className="flex flex-col gap-3 p-6">
    <div className="flex items-start justify-end gap-3">
      <div className="rounded-lg rounded-tr-none bg-muted px-4 py-2.5">
        <p className="text-foreground text-sm">
          Make the intro more conversational and add a stronger hook.
        </p>
      </div>
    </div>

    <div className="flex items-start gap-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted font-semibold text-foreground text-xs">
        AI
      </div>
      <div className="flex-1 space-y-2">
        <div className="rounded-lg rounded-tl-none border border-border/40 bg-muted/30 px-4 py-2.5">
          <p className="text-foreground text-sm">
            Here&apos;s the revised intro:
          </p>
          <div className="mt-2 rounded-md border border-foreground/8 bg-muted/50 p-3">
            <p className="text-foreground text-sm italic">
              &quot;What if I told you that 73% of SaaS companies waste their
              content budget on posts nobody reads?&quot;
            </p>
            <span className="inline-block h-4 w-0.5 animate-pulse bg-primary" />
          </div>
        </div>
      </div>
    </div>
  </div>
);

const PenLineIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zM19.5 12v7.5a1.5 1.5 0 01-1.5 1.5H5.25a1.5 1.5 0 01-1.5-1.5V6.75a1.5 1.5 0 011.5-1.5H12"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const ThreadIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M20.25 8.511c.884.284 1.5 1.128 1.5 2.097v4.286c0 1.136-.847 2.1-1.98 2.193-.34.027-.68.052-1.02.072v3.091l-3-3c-1.354 0-2.694-.055-4.02-.163a2.115 2.115 0 01-.825-.242m9.345-8.334a2.126 2.126 0 00-.476-.095 48.64 48.64 0 00-8.048 0c-1.131.094-1.976 1.057-1.976 2.192v4.286c0 .837.46 1.58 1.155 1.951m9.345-8.334V6.637c0-1.621-1.152-3.026-2.76-3.235A48.455 48.455 0 0011.25 3c-2.115 0-4.198.137-6.24.402-1.608.209-2.76 1.614-2.76 3.235v6.226c0 1.621 1.152 3.026 2.76 3.235.577.075 1.157.14 1.74.194V21l4.155-4.155"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const MailIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const NewspaperIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M12 7.5h1.5m-1.5 3h1.5m-7.5 3h7.5m-7.5 3h7.5m3-9h3.375c.621 0 1.125.504 1.125 1.125V18a2.25 2.25 0 01-2.25 2.25M16.5 7.5V18a2.25 2.25 0 002.25 2.25M16.5 7.5V4.875c0-.621-.504-1.125-1.125-1.125H4.125C3.504 3.75 3 4.254 3 4.875V18a2.25 2.25 0 002.25 2.25h13.5M6 7.5h3v3H6v-3z"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const MegaphoneIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M10.34 15.84c-.688-.06-1.386-.09-2.09-.09H7.5a4.5 4.5 0 110-9h.75c.704 0 1.402-.03 2.09-.09m0 9.18c.253.962.584 1.892.985 2.783.247.55.06 1.21-.463 1.511l-.657.38c-.551.318-1.26.117-1.527-.461a20.845 20.845 0 01-1.44-4.282m3.102.069a18.03 18.03 0 01-.59-4.59c0-1.586.205-3.124.59-4.59m0 9.18a23.848 23.848 0 018.835 2.535M10.34 6.66a23.847 23.847 0 008.835-2.535m0 0A23.74 23.74 0 0018.795 3m.38 1.125a23.91 23.91 0 011.014 5.395m-1.014 8.855c-.118.38-.245.754-.38 1.125m.38-1.125a23.91 23.91 0 001.014-5.395m0-3.46c.495.413.811 1.035.811 1.73 0 .695-.316 1.317-.811 1.73m0-3.46a24.347 24.347 0 010 3.46"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const DocumentTextIcon = () => (
  <svg
    className="size-5 text-foreground"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

/** Visual 4 — Content type selector grid */
export const InterviewVisual4 = () => {
  const types = [
    { label: "Blog Post", icon: <PenLineIcon /> },
    { label: "Thread", icon: <ThreadIcon /> },
    { label: "Email", icon: <MailIcon /> },
    { label: "Newsletter", icon: <NewspaperIcon /> },
    { label: "Ad Copy", icon: <MegaphoneIcon /> },
    { label: "Press Release", icon: <DocumentTextIcon /> },
  ];

  return (
    <div className="flex flex-col gap-4 p-6">
      <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
        Choose a content type
      </p>
      <div className="grid grid-cols-3 gap-2">
        {types.map((t, i) => (
          <button
            className={`flex flex-col items-center gap-1.5 rounded-lg border p-3 text-center transition-all duration-200 ${
              i === 0
                ? "border-foreground/10 bg-muted/50 shadow-sm"
                : "border-border/40 bg-background hover:border-foreground/20/20"
            }`}
            key={t.label}
            type="button"
          >
            {t.icon}
            <span className="font-medium text-foreground text-xs">
              {t.label}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
};
