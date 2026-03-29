import { HugeiconsIcon } from "@hugeicons/react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@strait/ui/components/command";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useEffect, useState } from "react";
import {
  AlertIcon,
  BriefcaseIcon,
  ClockIcon,
  DashboardIcon,
  FileTextIcon,
  LayersIcon,
  PlayActionIcon,
  SearchIcon,
  SettingsOutlineIcon,
  SparklesIcon,
  TrendingUpIcon,
  UserIcon,
  WebhookIcon,
  WorkflowIcon,
} from "@/lib/icons";

type CommandRoute = {
  title: string;
  icon: typeof DashboardIcon;
  to: string;
  keywords?: string;
};

const navigationRoutes: CommandRoute[] = [
  { title: "Overview", icon: DashboardIcon, to: "/app", keywords: "home" },
  {
    title: "Dashboard",
    icon: TrendingUpIcon,
    to: "/app/dashboard",
    keywords: "analytics stats metrics",
  },
  {
    title: "Jobs",
    icon: BriefcaseIcon,
    to: "/app/jobs",
    keywords: "tasks cron scheduled",
  },
  {
    title: "Agents",
    icon: SparklesIcon,
    to: "/app/agents",
    keywords: "ai llm assistants",
  },
  {
    title: "Workflows",
    icon: WorkflowIcon,
    to: "/app/workflows",
    keywords: "pipelines dag",
  },
  {
    title: "Runs",
    icon: PlayActionIcon,
    to: "/app/runs",
    keywords: "executions history",
  },
  {
    title: "Schedules",
    icon: ClockIcon,
    to: "/app/schedules",
    keywords: "cron timer recurring",
  },
  {
    title: "Dead Letter",
    icon: AlertIcon,
    to: "/app/dlq",
    keywords: "failed errors queue",
  },
  {
    title: "Logs",
    icon: FileTextIcon,
    to: "/app/logs",
    keywords: "output debug",
  },
  {
    title: "Events",
    icon: LayersIcon,
    to: "/app/events",
    keywords: "triggers webhooks",
  },
  {
    title: "Webhooks",
    icon: WebhookIcon,
    to: "/app/webhooks",
    keywords: "subscriptions callbacks",
  },
];

interface CommandMenuProps {
  organizationId?: string;
}

const CommandMenu = ({ organizationId }: CommandMenuProps) => {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((prev) => !prev);
      }
    };
    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  const handleSelect = useCallback(
    (to: string) => {
      setOpen(false);
      navigate({ to });
    },
    [navigate]
  );

  const orgSettingsRoute = organizationId
    ? `/app/org/${organizationId}`
    : "/app/settings";
  const routes = navigationRoutes;

  return (
    <>
      <button
        className="flex h-8 w-full items-center gap-2 rounded-lg border border-sidebar-border bg-sidebar-accent/50 px-2 text-muted-foreground text-sm transition-colors hover:bg-sidebar-accent"
        onClick={() => setOpen(true)}
        type="button"
      >
        <HugeiconsIcon className="size-4" icon={SearchIcon} />
        <span className="flex-1 text-left">Search...</span>
        <kbd className="pointer-events-none hidden rounded border bg-muted px-1.5 font-mono text-[10px] text-muted-foreground sm:inline-block">
          ⌘K
        </kbd>
      </button>

      <CommandDialog onOpenChange={setOpen} open={open}>
        <CommandInput placeholder="Search for pages, jobs, runs..." />
        <CommandList>
          <CommandEmpty>No results found.</CommandEmpty>

          <CommandGroup heading="Navigation">
            {routes.map((route) => (
              <CommandItem
                key={route.to}
                keywords={route.keywords?.split(" ")}
                onSelect={() => handleSelect(route.to)}
              >
                <HugeiconsIcon
                  className="size-4 text-muted-foreground"
                  icon={route.icon}
                />
                {route.title}
              </CommandItem>
            ))}
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Settings">
            <CommandItem
              keywords={["profile", "password", "email", "account"]}
              onSelect={() => handleSelect("/app/settings")}
            >
              <HugeiconsIcon
                className="size-4 text-muted-foreground"
                icon={UserIcon}
              />
              Account Settings
            </CommandItem>
            <CommandItem
              keywords={["org", "team", "billing", "subscription", "members"]}
              onSelect={() => handleSelect(orgSettingsRoute)}
            >
              <HugeiconsIcon
                className="size-4 text-muted-foreground"
                icon={SettingsOutlineIcon}
              />
              Organization Settings
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Quick Actions">
            <CommandItem
              keywords={["new", "create", "add"]}
              onSelect={() => handleSelect("/app/jobs")}
            >
              <HugeiconsIcon
                className="size-4 text-muted-foreground"
                icon={BriefcaseIcon}
              />
              Create Job
              <CommandShortcut>⌘N</CommandShortcut>
            </CommandItem>
            <CommandItem
              keywords={["new", "create", "add", "pipeline"]}
              onSelect={() => handleSelect("/app/workflows")}
            >
              <HugeiconsIcon
                className="size-4 text-muted-foreground"
                icon={WorkflowIcon}
              />
              Create Workflow
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
};

export default CommandMenu;
