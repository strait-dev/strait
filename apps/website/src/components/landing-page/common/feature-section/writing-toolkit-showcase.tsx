import {
  BookEditIcon,
  DownloadSquare02Icon,
  Folder01Icon,
  TextBoldIcon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls.ts";
import FeatureShowcase from "./feature-showcase.tsx";
import { ContentTypesVisual2 } from "./visuals/content-types-visual.tsx";
import { EditorVisual1, EditorVisual2 } from "./visuals/editor-visual.tsx";

const ExportVisual = () => {
  const formats = [
    {
      name: "Run Events",
      ext: "events",
      abbr: "EVT",
      desc: "Structured execution timeline",
    },
    {
      name: "Usage Data",
      ext: "usage",
      abbr: "USD",
      desc: "Token and cost tracking",
    },
    {
      name: "Debug Bundle",
      ext: "debug",
      abbr: "DBG",
      desc: "Run diagnostics and artifacts",
    },
    {
      name: "Replay Payload",
      ext: "replay",
      abbr: "RPL",
      desc: "Re-trigger with controlled state",
    },
  ];

  return (
    <div className="flex flex-col gap-4 p-6">
      <p className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
        Operational outputs
      </p>

      <div className="rounded-lg border border-border/40 bg-background p-1">
        {formats.map((fmt, i) => (
          <div
            className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors ${
              i === 0 ? "bg-muted/50" : "hover:bg-muted/50"
            }`}
            key={fmt.ext}
          >
            <span className="flex size-8 items-center justify-center rounded-md bg-muted text-foreground text-xs">
              {fmt.abbr}
            </span>
            <div className="flex-1">
              <p className="font-medium text-foreground text-sm">{fmt.name}</p>
              <p className="text-muted-foreground text-xs">{fmt.desc}</p>
            </div>
            <span className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs">
              {fmt.ext}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

const WritingToolkitShowcase = () => (
  <FeatureShowcase
    className="border-border/40 border-y bg-muted/20"
    cta={{
      href: dashboardHref("/login"),
      label: "Explore control tools",
    }}
    description="Give your team the visibility and controls they need to keep delivery moving every day."
    features={[
      {
        title: "Track what each run is doing",
        description:
          "Follow progress and outcomes in real time so nothing gets lost in the background.",
        icon: TextBoldIcon,
      },
      {
        title: "See issues before they spread",
        description:
          "Use one operational view to spot slowdowns and troubleshoot faster.",
        icon: Folder01Icon,
      },
      {
        title: "Replay failed work in seconds",
        description:
          "Recover runs quickly and keep teams focused on shipping instead of patching.",
        icon: DownloadSquare02Icon,
      },
      {
        title: "Keep usage predictable",
        description:
          "Set clear limits to protect costs as workflows and teams grow.",
        icon: BookEditIcon,
      },
    ]}
    title="Operate with confidence once workflows are live"
    visuals={[
      <EditorVisual1 key="wt-editor" />,
      <EditorVisual2 key="wt-folders" />,
      <ExportVisual key="wt-export" />,
      <ContentTypesVisual2 key="wt-content" />,
    ]}
  />
);

export default WritingToolkitShowcase;
