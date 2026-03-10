import {
  Briefcase01Icon,
  PencilEdit02Icon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons";

import AudienceVisuals from "./audience-visuals";

const ICON_MAP: Record<string, typeof PencilEdit02Icon> = {
  "pencil-edit-02": PencilEdit02Icon,
  "user-group": UserGroupIcon,
  "briefcase-01": Briefcase01Icon,
};

type ExampleItem = {
  _id: string;
  text: string;
};

type AudienceItem = {
  _id: string;
  title: string;
  description: string;
  icon_name: string;
  examples?: {
    items?: ExampleItem[];
  };
};

const AUDIENCES: AudienceItem[] = [
  {
    _id: "backend",
    title: "Backend engineering teams",
    description:
      "Ship reliable background execution with retries, idempotency, and terminal-state control built into the runtime.",
    icon_name: "pencil-edit-02",
    examples: {
      items: [
        { _id: "b1", text: "Webhook consumers with DLQ replay" },
        { _id: "b2", text: "Scheduled maintenance and cleanup jobs" },
      ],
    },
  },
  {
    _id: "platform",
    title: "Platform and SRE teams",
    description:
      "Standardize job infrastructure across services with one API, one worker model, and one observability surface.",
    icon_name: "user-group",
    examples: {
      items: [
        { _id: "p1", text: "Unified run lifecycle across all projects" },
        { _id: "p2", text: "Operational dashboards for queue and run health" },
      ],
    },
  },
  {
    _id: "agents",
    title: "AI agent builders",
    description:
      "Track token usage, enforce budgets, and orchestrate long-running agent workflows with checkpoints and approvals.",
    icon_name: "briefcase-01",
    examples: {
      items: [
        { _id: "a1", text: "Multi-step agent DAGs with human gates" },
        { _id: "a2", text: "Per-run and daily cost budget enforcement" },
      ],
    },
  },
];

const AudienceSection = () => {
  const headingId = "audience-title";

  return (
    <section aria-labelledby={headingId} className="py-20 sm:py-28">
      <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="mb-14 max-w-3xl animate-on-scroll">
          <h2
            className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            <span className="font-bold text-foreground">
              Built for teams that need dependable execution every day.
            </span>{" "}
            <span className="text-muted-foreground">
              Whether you run product workflows, platform jobs, or agent flows,
              Strait keeps operations simple.
            </span>
          </h2>
        </div>

        <AudienceVisuals audiences={AUDIENCES} iconMap={ICON_MAP} />
      </div>
    </section>
  );
};

export default AudienceSection;
