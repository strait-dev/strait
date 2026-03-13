/* ------------------------------------------------------------------ */
/*  Content Types Showcase — 4 animated mock-UI visuals               */
/* ------------------------------------------------------------------ */

/** Visual 1 — Blog post outline */
export const ContentTypesVisual1 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Blog post structure
    </p>

    <div className="rounded-lg border border-border/40 bg-background p-4">
      <div className="space-y-3">
        {[
          {
            section: "Title",
            content: "Scaling Content Marketing in 2025",
            level: 0,
          },
          {
            section: "Introduction",
            content: "Hook + problem statement + thesis",
            level: 0,
          },
          {
            section: "Key Point 1",
            content: "Why traditional content fails",
            level: 1,
          },
          {
            section: "Key Point 2",
            content: "The conversation-first approach",
            level: 1,
          },
          {
            section: "Key Point 3",
            content: "Measuring content ROI",
            level: 1,
          },
          {
            section: "Case Study",
            content: "Real example with results",
            level: 0,
          },
          { section: "Conclusion", content: "Summary + CTA", level: 0 },
        ].map((item, i) => (
          <div
            className={`flex items-center gap-2 ${item.level > 0 ? "ml-4" : ""}`}
            key={item.section}
          >
            <span
              className={`font-mono text-xs ${i === 0 ? "text-foreground" : "text-muted-foreground"}`}
            >
              {item.level > 0 ? "├─" : "##"}
            </span>
            <div className="flex-1">
              <span className="font-medium text-foreground text-sm">
                {item.section}
              </span>
              <span className="ml-2 text-muted-foreground text-xs">
                {item.content}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  </div>
);

/** Visual 2 — Twitter thread mockup */
export const ContentTypesVisual2 = () => (
  <div className="flex flex-col gap-3 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Twitter thread
    </p>

    <div className="space-y-0">
      {[
        {
          num: 1,
          text: "We grew our content traffic 340% in 6 months. Here's the playbook:",
          chars: "73/280",
        },
        {
          num: 2,
          text: "Step 1: Stop writing for search engines. Start writing for humans who have specific problems.",
          chars: "94/280",
        },
        {
          num: 3,
          text: "Step 2: Use AI interviews to uncover angles you'd never think of on your own.",
          chars: "78/280",
        },
      ].map((tweet, i) => (
        <div
          className="relative border-foreground/8 border-l-2 py-2 pl-4"
          key={tweet.num}
        >
          {i < 2 && (
            <div className="absolute bottom-0 left-[-1px] h-2 w-0.5 bg-muted" />
          )}
          <div className="rounded-lg border border-border/40 bg-background p-3">
            <div className="mb-1.5 flex items-center justify-between">
              <span className="rounded-full bg-muted px-2 py-0.5 font-semibold text-foreground text-xs">
                {tweet.num}/{3}
              </span>
              <span className="font-mono text-muted-foreground text-xs">
                {tweet.chars}
              </span>
            </div>
            <p className="text-foreground text-sm leading-relaxed">
              {tweet.text}
            </p>
          </div>
        </div>
      ))}
    </div>
  </div>
);

/** Visual 3 — Email template */
export const ContentTypesVisual3 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Email template
    </p>

    <div className="overflow-hidden rounded-lg border border-border/40 bg-background">
      {/* Email header fields */}
      <div className="space-y-2 border-border/40 border-b p-3">
        <div className="flex items-center gap-2">
          <span className="w-12 text-right text-muted-foreground text-xs">
            To:
          </span>
          <span className="text-foreground text-sm">team@company.com</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="w-12 text-right text-muted-foreground text-xs">
            Subject:
          </span>
          <span className="font-medium text-foreground text-sm">
            Quick follow-up on our content strategy
          </span>
        </div>
      </div>

      {/* Email body */}
      <div className="space-y-2 p-4">
        <p className="text-foreground text-sm">Hi Sarah,</p>
        <p className="text-muted-foreground text-sm leading-relaxed">
          Following up on our conversation about the Q1 content plan. I&apos;ve
          drafted three approaches we could take...
        </p>
        <div className="space-y-1">
          <div className="h-2 w-full rounded bg-foreground/8" />
          <div className="h-2 w-9/12 rounded bg-foreground/8" />
        </div>
        <p className="mt-3 text-foreground text-sm">
          Best,
          <br />
          Alex
        </p>
      </div>
    </div>
  </div>
);

/** Visual 4 — Press release structure */
export const ContentTypesVisual4 = () => (
  <div className="flex flex-col gap-4 p-6">
    <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
      Press release
    </p>

    <div className="space-y-3 rounded-lg border border-border/40 bg-background p-4">
      {/* Headline */}
      <div className="border-border/40 border-b pb-2">
        <p className="text-muted-foreground text-xs uppercase tracking-wider">
          FOR IMMEDIATE RELEASE
        </p>
        <h4 className="mt-1 font-bold text-foreground text-sm">
          Strait Launches AI-Powered Writing Assistant for Content Teams
        </h4>
        <p className="mt-0.5 text-muted-foreground text-xs italic">
          Conversation-first approach generates 3x more draft variations
        </p>
      </div>

      {/* Body preview */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground text-xs">
            Lead
          </span>
          <div className="h-1.5 flex-1 rounded bg-foreground/8" />
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs">
            Body
          </span>
          <div className="h-1.5 flex-1 rounded bg-foreground/8" />
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs">
            Quote
          </span>
          <div className="h-1.5 flex-1 rounded bg-foreground/8" />
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs">
            Contact
          </span>
          <div className="h-1.5 flex-1 rounded bg-foreground/8" />
        </div>
      </div>
    </div>
  </div>
);
