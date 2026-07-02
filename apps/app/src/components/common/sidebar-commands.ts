import type { ProjectPermissionFlags } from "@/hooks/auth/use-project-permissions";
import {
  AlertIcon,
  BriefcaseIcon,
  ClockIcon,
  DashboardIcon,
  FileTextIcon,
  LayersIcon,
  PlayActionIcon,
  SettingsOutlineIcon,
  SparklesIcon,
  TrendingUpIcon,
  UserIcon,
  WebhookIcon,
  WorkflowIcon,
} from "@/lib/icons";
import {
  jobResourcePermissions,
  workflowResourcePermissions,
} from "@/lib/resource-permissions";

export type NavItem = {
  title: string;
  url: string;
  icon: typeof DashboardIcon;
  exact?: boolean;
};

export type SidebarCommandItem = {
  label: string;
  href?: string;
  url?: string;
  icon?: typeof DashboardIcon;
  shortcut?: string;
  keywords?: string[];
};

export type SidebarCommandGroup = {
  heading: string;
  items: SidebarCommandItem[];
};

export const mainNav: NavItem[] = [
  {
    title: "Getting Started",
    url: "/app",
    icon: SparklesIcon,
    exact: true,
  },
  { title: "Dashboard", url: "/app/dashboard", icon: DashboardIcon },
  { title: "Analytics", url: "/app/analytics", icon: TrendingUpIcon },
  { title: "Jobs", url: "/app/jobs", icon: BriefcaseIcon },
  { title: "Workflows", url: "/app/workflows", icon: WorkflowIcon },
  { title: "Runs", url: "/app/runs", icon: PlayActionIcon },
  { title: "Schedules", url: "/app/schedules", icon: ClockIcon },
  { title: "Dead Letter", url: "/app/dlq", icon: AlertIcon },
];

export const observabilityNav: NavItem[] = [
  { title: "Logs", url: "/app/logs", icon: FileTextIcon },
  { title: "Events", url: "/app/events", icon: LayersIcon },
  { title: "Webhooks", url: "/app/webhooks", icon: WebhookIcon },
];

export function buildQuickCreateCommands(
  permissions: ProjectPermissionFlags
): SidebarCommandItem[] {
  const jobActions = jobResourcePermissions(permissions);
  const workflowActions = workflowResourcePermissions(permissions);
  const items: SidebarCommandItem[] = [];

  if (jobActions.canCreate) {
    items.push(
      {
        label: "Create job",
        href: "/app/jobs?create=1",
        icon: BriefcaseIcon,
        shortcut: "⌘N",
        keywords: ["new", "create", "add"],
      },
      {
        label: "Create schedule",
        href: "/app/schedules?create=1",
        icon: ClockIcon,
        keywords: ["new", "create", "add", "cron"],
      }
    );
  }

  if (workflowActions.canCreate) {
    items.push({
      label: "Create workflow",
      href: "/app/workflows?create=1",
      icon: WorkflowIcon,
      keywords: ["new", "create", "add", "pipeline"],
    });
  }

  return items;
}

export function buildSidebarCommandGroups(
  permissions: ProjectPermissionFlags,
  orgSettingsRoute: string
): SidebarCommandGroup[] {
  const groups: SidebarCommandGroup[] = [
    {
      heading: "Navigation",
      items: [...mainNav, ...observabilityNav].map((item) => ({
        label: item.title,
        url: item.url,
        icon: item.icon,
        keywords: [item.title.toLowerCase()],
      })),
    },
    {
      heading: "Settings",
      items: [
        {
          label: "Account Settings",
          url: "/app/settings",
          icon: UserIcon,
          keywords: ["profile", "password", "email", "account"],
        },
        {
          label: "Organization Settings",
          url: orgSettingsRoute,
          icon: SettingsOutlineIcon,
          keywords: ["org", "team", "billing", "subscription", "members"],
        },
      ],
    },
  ];
  const quickActions = buildQuickCreateCommands(permissions);

  if (quickActions.length > 0) {
    groups.push({
      heading: "Quick Actions",
      items: quickActions,
    });
  }

  return groups;
}

export function commandValue(item: SidebarCommandItem) {
  return item.keywords ? [item.label, ...item.keywords].join(" ") : item.label;
}
