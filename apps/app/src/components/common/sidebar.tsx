import { HugeiconsIcon } from "@hugeicons/react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@strait/ui/components/collapsible";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@strait/ui/components/sidebar";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouterState } from "@tanstack/react-router";
import { Suspense, useState } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import {
  AlertIcon,
  BriefcaseIcon,
  ChevronDownIcon,
  ClockIcon,
  DashboardIcon,
  FileTextIcon,
  LayersIcon,
  PlayActionIcon,
  TrendingUpIcon,
  WebhookIcon,
  WorkflowIcon,
} from "@/lib/icons";
import type { Session } from "@/routes/__root";
import OrganizationDropdownMenu from "../organization/organization-dropdown-menu";
import ProjectSwitcher from "../project/project-switcher";
import PaymentPendingCard from "../subscription/payment-pending-card";
import TrialUpgradeCard from "../subscription/trial-upgrade-card";
import CommandMenu from "./command-menu";

type NavItem = {
  title: string;
  url: string;
  icon: typeof DashboardIcon;
  /** When true, only highlight on exact match (e.g. `/app` but not `/app/jobs`). */
  exact?: boolean;
};

const mainNav: NavItem[] = [
  { title: "Overview", url: "/app", icon: DashboardIcon, exact: true },
  { title: "Dashboard", url: "/app/dashboard", icon: TrendingUpIcon },
  { title: "Jobs", url: "/app/jobs", icon: BriefcaseIcon },
  { title: "Workflows", url: "/app/workflows", icon: WorkflowIcon },
  { title: "Runs", url: "/app/runs", icon: PlayActionIcon },
  { title: "Schedules", url: "/app/schedules", icon: ClockIcon },
  { title: "Dead Letter", url: "/app/dlq", icon: AlertIcon },
];

const observabilityNav: NavItem[] = [
  { title: "Logs", url: "/app/logs", icon: FileTextIcon },
  { title: "Events", url: "/app/events", icon: LayersIcon },
  { title: "Webhooks", url: "/app/webhooks", icon: WebhookIcon },
];

type Environment = "production" | "staging" | "development";

const environments: { value: Environment; label: string; dotClass: string }[] =
  [
    {
      value: "production",
      label: "Production",
      dotClass: "bg-green-500",
    },
    {
      value: "staging",
      label: "Staging",
      dotClass: "bg-blue-500",
    },
    {
      value: "development",
      label: "Development",
      dotClass: "bg-yellow-500",
    },
  ];

type Props = {
  session: NonNullable<Session>;
};

const AppSidebar = ({ session }: Props) => {
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { shouldShowUpgrade, hasPendingPayment } = subscriptionState;

  const [environment, setEnvironment] = useState<Environment>("production");
  const currentEnv =
    environments.find((e) => e.value === environment) ?? environments[0];

  const pathname = useRouterState({
    select: (s) => s.location.pathname,
  });

  /** Check whether a nav item is active based on the current pathname. */
  const isActive = (item: NavItem) => {
    if (item.exact) {
      return pathname === item.url;
    }
    // Match the item's url and any nested routes beneath it.
    return pathname === item.url || pathname.startsWith(`${item.url}/`);
  };

  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader className="h-16 border-sidebar-border border-b">
        <div className="flex h-full w-full items-center justify-between px-2">
          <Link to="/app">
            <span className="sr-only">Strait</span>
            <img
              alt="Strait logo"
              className="h-8 w-auto"
              height={20}
              src="/strait.svg"
              width={20}
            />
          </Link>

          <DropdownMenu>
            <DropdownMenuTrigger className="flex items-center gap-1.5 rounded-md border px-2 py-1 font-medium text-sidebar-foreground text-xs hover:bg-sidebar-accent">
              <span
                className={`inline-block size-2 rounded-full ${currentEnv.dotClass}`}
              />
              {currentEnv.label}
              <HugeiconsIcon
                className="size-3 text-muted-foreground"
                icon={ChevronDownIcon}
              />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" sideOffset={4}>
              {environments.map((env) => (
                <DropdownMenuItem
                  key={env.value}
                  onClick={() => setEnvironment(env.value)}
                >
                  <span
                    className={`inline-block size-2 rounded-full ${env.dotClass}`}
                  />
                  {env.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </SidebarHeader>

      <SidebarContent>
        {/* Project switcher */}
        <SidebarGroup>
          <SidebarGroupLabel>Project</SidebarGroupLabel>
          <SidebarMenu>
            <SidebarMenuItem>
              <ProjectSwitcher user={session.user} />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {/* Search */}
        <SidebarGroup>
          <CommandMenu organizationId={session.user.defaultOrganizationId} />
        </SidebarGroup>

        {/* Main navigation */}
        <SidebarGroup>
          <SidebarMenu>
            {mainNav.map((item) => (
              <SidebarMenuItem key={item.url}>
                <SidebarMenuButton
                  isActive={isActive(item)}
                  render={<Link to={item.url} />}
                  tooltip={item.title}
                >
                  <HugeiconsIcon
                    className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
                    icon={item.icon}
                    size={22}
                  />
                  <span>{item.title}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroup>

        {/* Observability group */}
        <SidebarGroup>
          <Collapsible className="group/collapsible" defaultOpen>
            <SidebarGroupLabel
              render={<CollapsibleTrigger className="w-full" />}
            >
              Observability
              <HugeiconsIcon
                className="ml-auto size-4 transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                icon={ChevronDownIcon}
              />
            </SidebarGroupLabel>
            <CollapsibleContent>
              <SidebarGroupContent>
                <SidebarMenu>
                  {observabilityNav.map((item) => (
                    <SidebarMenuItem key={item.url}>
                      <SidebarMenuButton
                        isActive={isActive(item)}
                        render={<Link to={item.url} />}
                        tooltip={item.title}
                      >
                        <HugeiconsIcon
                          className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
                          icon={item.icon}
                          size={22}
                        />
                        <span>{item.title}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </CollapsibleContent>
          </Collapsible>
        </SidebarGroup>
      </SidebarContent>

      {hasPendingPayment ? <PaymentPendingCard /> : null}
      {shouldShowUpgrade ? <TrialUpgradeCard /> : null}

      <SidebarFooter className="flex flex-col border-sidebar-border border-t">
        <Suspense
          fallback={
            <SidebarMenuButton className="w-full" size="lg">
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-normal">Loading...</span>
              </div>
            </SidebarMenuButton>
          }
        >
          <OrganizationDropdownMenu session={session} user={session.user} />
        </Suspense>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
};

export default AppSidebar;
