import { PencilEdit02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useEffect, useRef } from "react";

const WriterVisual = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div
      className="audience-card-el mb-1 flex items-center gap-2"
      style={{ "--el-delay": "0.2s" } as React.CSSProperties}
    >
      <div className="flex gap-1">
        <span className="size-2 rounded-full bg-primary-foreground/30" />
        <span className="size-2 rounded-full bg-primary-foreground/30" />
        <span className="size-2 rounded-full bg-primary-foreground/30" />
      </div>
      <span className="text-primary-foreground/40 text-xs">essay-draft.md</span>
    </div>

    <div
      className="audience-card-el space-y-2"
      style={{ "--el-delay": "0.4s" } as React.CSSProperties}
    >
      <div className="h-3.5 w-[55%] rounded bg-primary-foreground/25" />
      <div className="h-2.5 w-full rounded bg-primary-foreground/15" />
      <div className="h-2.5 w-[90%] rounded bg-primary-foreground/15" />
      <div className="h-2.5 w-[75%] rounded bg-primary-foreground/15" />
    </div>

    <div
      className="audience-card-el mt-2 flex items-center gap-2"
      style={{ "--el-delay": "0.8s" } as React.CSSProperties}
    >
      <div className="rounded-md border border-primary-foreground/15 bg-primary-foreground/10 px-2.5 py-1">
        <span className="text-primary-foreground/60 text-xs">
          AI: Tighten your opening
        </span>
      </div>
    </div>

    <div
      className="audience-card-el mt-1"
      style={{ "--el-delay": "1.1s" } as React.CSSProperties}
    >
      <span className="audience-cursor inline-block h-4 w-0.5 bg-primary-foreground/60" />
    </div>
  </div>
);

const TeamVisual = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div
      className="audience-card-el flex items-center gap-2"
      style={{ "--el-delay": "0.2s" } as React.CSSProperties}
    >
      <div className="size-6 rounded-md bg-primary-foreground/20" />
      <span className="font-medium text-primary-foreground/60 text-xs">
        Content Team
      </span>
    </div>

    {[
      { name: "Sarah", doc: "Q1 Blog Strategy", status: "Editing" },
      { name: "James", doc: "Product Launch Brief", status: "Review" },
      { name: "Maya", doc: "Case Study: Acme", status: "Final" },
    ].map((member, i) => (
      <div
        className="audience-card-el flex items-center justify-between rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 px-3 py-2"
        key={member.name}
        style={{ "--el-delay": `${0.5 + i * 0.2}s` } as React.CSSProperties}
      >
        <div className="flex items-center gap-2">
          <div className="flex size-5 items-center justify-center rounded-full bg-primary-foreground/15">
            <span className="font-medium text-[9px] text-primary-foreground/50">
              {member.name[0]}
            </span>
          </div>
          <div className="flex flex-col">
            <span className="text-primary-foreground/70 text-xs leading-tight">
              {member.doc}
            </span>
            <span className="text-[10px] text-primary-foreground/40">
              {member.name}
            </span>
          </div>
        </div>
        <span className="rounded-md border border-primary-foreground/15 bg-primary-foreground/10 px-2 py-0.5 text-[10px] text-primary-foreground/50">
          {member.status}
        </span>
      </div>
    ))}
  </div>
);

const BusinessVisual = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div
      className="audience-card-el flex items-center justify-between"
      style={{ "--el-delay": "0.2s" } as React.CSSProperties}
    >
      <span className="font-medium text-primary-foreground/60 text-xs">
        Style Profiles
      </span>
      <span className="text-[10px] text-primary-foreground/40">3 active</span>
    </div>

    {[
      { name: "Formal Reports", tone: "Professional, data-driven" },
      { name: "Client Emails", tone: "Warm, concise" },
      { name: "Marketing Copy", tone: "Persuasive, active voice" },
    ].map((profile, i) => (
      <div
        className="audience-card-el rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 px-3 py-2.5"
        key={profile.name}
        style={{ "--el-delay": `${0.4 + i * 0.2}s` } as React.CSSProperties}
      >
        <div className="flex items-center justify-between">
          <span className="font-medium text-primary-foreground/70 text-xs">
            {profile.name}
          </span>
          <div className="size-1.5 rounded-full bg-primary-foreground/30" />
        </div>
        <span className="mt-0.5 block text-[10px] text-primary-foreground/40">
          {profile.tone}
        </span>
      </div>
    ))}

    <div
      className="audience-card-el mt-1 flex items-center gap-1.5"
      style={{ "--el-delay": "1.1s" } as React.CSSProperties}
    >
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-primary-foreground/10">
        <div className="h-full w-[85%] rounded-full bg-primary-foreground/30" />
      </div>
      <span className="text-[10px] text-primary-foreground/40">Consistent</span>
    </div>
  </div>
);

const VISUALS: Record<string, React.FC> = {
  "pencil-edit-02": WriterVisual,
  "user-group": TeamVisual,
  "briefcase-01": BusinessVisual,
};

type AudienceItem = {
  _id: string;
  title: string;
  description: string;
  icon_name: string;
  examples?: {
    items?: { _id: string; text: string }[];
  };
};

type AudienceVisualsProps = {
  audiences: AudienceItem[];
  iconMap: Record<string, typeof PencilEdit02Icon>;
};

const AudienceVisuals = ({ audiences, iconMap }: AudienceVisualsProps) => {
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
      { threshold: 0.1 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <div
      className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 lg:gap-8"
      ref={sectionRef}
    >
      {audiences.map((audience) => {
        const IconComponent = iconMap[audience.icon_name] ?? PencilEdit02Icon;
        const Visual = VISUALS[audience.icon_name];
        const examples = audience.examples?.items ?? [];

        return (
          <div className="flex flex-col" key={audience._id}>
            <div className="relative aspect-square overflow-hidden rounded-2xl bg-primary">
              <div className="showcase-dots pointer-events-none absolute inset-0" />
              <div
                className="pointer-events-none absolute inset-0 opacity-30"
                style={{
                  background:
                    "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)",
                }}
              />
              <div className="relative z-10 h-full">
                {Visual ? (
                  <Visual />
                ) : (
                  <div className="flex h-full items-center justify-center">
                    <HugeiconsIcon
                      className="size-10 text-primary-foreground/40"
                      icon={IconComponent}
                    />
                  </div>
                )}
              </div>
            </div>

            <div className="mt-5">
              <div className="flex items-center gap-3">
                <div className="icon-chip">
                  <HugeiconsIcon
                    className="size-4 text-foreground"
                    icon={IconComponent}
                  />
                </div>
                <h3 className="font-semibold text-foreground text-lg">
                  {audience.title}
                </h3>
              </div>
              <p className="mt-2 text-pretty text-base text-muted-foreground leading-relaxed">
                {audience.description}
              </p>

              {examples.length > 0 ? (
                <div className="mt-4 flex flex-wrap gap-2">
                  {examples.map((example) => (
                    <span
                      className="rounded-md bg-muted/60 px-2.5 py-1 text-foreground text-xs"
                      key={example._id}
                    >
                      {example.text}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
          </div>
        );
      })}
    </div>
  );
};

export default AudienceVisuals;
