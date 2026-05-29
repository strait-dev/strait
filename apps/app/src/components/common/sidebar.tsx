import { HugeiconsIcon } from "@hugeicons/react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@strait/ui/components/collapsible";
import {
  CommandMenu,
  type CommandMenuGroup,
} from "@strait/ui/components/command-menu";
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
import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import { Suspense, useMemo } from "react";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { isCommunityEdition } from "@/lib/edition";
import {
  AlertIcon,
  BriefcaseIcon,
  ChevronDownIcon,
  ClockIcon,
  CreditCardIcon,
  DashboardIcon,
  FileTextIcon,
  HelpCircleIcon,
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
import type { Session } from "@/routes/__root";
import OrganizationDropdownMenu from "../organization/organization-dropdown-menu";
import ProjectSwitcher from "../project/project-switcher";
import PaymentPendingCard from "../subscription/payment-pending-card";
import TrialUpgradeCard from "../subscription/trial-upgrade-card";

type NavItem = {
  title: string;
  url: string;
  icon: typeof DashboardIcon;
  /** When true, only highlight on exact match (e.g. `/app` but not `/app/jobs`). */
  exact?: boolean;
};

const mainNav: NavItem[] = [
  { title: "Dashboard", url: "/app/dashboard", icon: DashboardIcon },
  { title: "Analytics", url: "/app/analytics", icon: TrendingUpIcon },
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

const commandRoutes = [
  { title: "Overview", url: "/app", icon: DashboardIcon, keywords: ["home"] },
  ...mainNav.map((item) => ({
    title: item.title,
    url: item.url,
    icon: item.icon,
    keywords: [item.title.toLowerCase()],
  })),
  ...observabilityNav.map((item) => ({
    title: item.title,
    url: item.url,
    icon: item.icon,
    keywords: [item.title.toLowerCase()],
  })),
];

type Props = {
  session: NonNullable<Session>;
};

const AppSidebar = ({ session }: Props) => {
  const navigate = useNavigate();
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { shouldShowUpgrade, hasPendingPayment } = subscriptionState;

  const pathname = useRouterState({
    select: (s) => s.location.pathname,
  });
  const orgSettingsRoute = session.user.defaultOrganizationId
    ? `/app/org/${session.user.defaultOrganizationId}`
    : "/app/settings";

  const commandGroups = useMemo<CommandMenuGroup[]>(
    () => [
      {
        heading: "Navigation",
        items: commandRoutes.map((route) => ({
          label: route.title,
          icon: route.icon,
          keywords: route.keywords,
          onSelect: () => navigate({ to: route.url }),
        })),
      },
      {
        heading: "Settings",
        items: [
          {
            label: "Account Settings",
            icon: UserIcon,
            keywords: ["profile", "password", "email", "account"],
            onSelect: () => navigate({ to: "/app/settings" }),
          },
          {
            label: "Organization Settings",
            icon: SettingsOutlineIcon,
            keywords: ["org", "team", "billing", "subscription", "members"],
            onSelect: () => navigate({ to: orgSettingsRoute }),
          },
        ],
      },
      {
        heading: "Quick Actions",
        items: [
          {
            label: "Create Job",
            icon: BriefcaseIcon,
            shortcut: "⌘N",
            keywords: ["new", "create", "add"],
            onSelect: () => navigate({ to: "/app/jobs" }),
          },
          {
            label: "Create Workflow",
            icon: WorkflowIcon,
            keywords: ["new", "create", "add", "pipeline"],
            onSelect: () => navigate({ to: "/app/workflows" }),
          },
        ],
      },
    ],
    [navigate, orgSettingsRoute]
  );

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
        <div className="flex h-full w-full items-center px-2">
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
          <CommandMenu
            groups={commandGroups}
            placeholder="Search for pages, jobs, runs..."
            trigger={
              <button
                className="flex h-8 w-full items-center gap-2 rounded-lg border border-sidebar-border bg-sidebar-accent/50 px-2 text-muted-foreground text-sm transition-colors hover:bg-sidebar-accent"
                type="button"
              >
                <HugeiconsIcon className="size-4" icon={SearchIcon} />
                <span className="flex-1 text-left">Search...</span>
                <kbd className="pointer-events-none hidden rounded border bg-muted px-1.5 font-mono text-[10px] text-muted-foreground sm:inline-block">
                  ⌘K
                </kbd>
              </button>
            }
          />
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

        {/* Billing — cloud edition only. Self-host builds hide this
            entire section so users cannot reach Stripe checkout or
            the customer portal. See `src/lib/edition.ts`. */}
        {isCommunityEdition ? null : (
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  isActive={
                    pathname === "/app/billing" ||
                    pathname.startsWith("/app/billing/")
                  }
                  render={<Link to="/app/billing" />}
                  tooltip="Billing"
                >
                  <HugeiconsIcon
                    className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
                    icon={CreditCardIcon}
                    size={22}
                  />
                  <span>Billing</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>

      {!isCommunityEdition && hasPendingPayment ? <PaymentPendingCard /> : null}
      {!isCommunityEdition && shouldShowUpgrade ? <TrialUpgradeCard /> : null}

      <SidebarFooter className="flex flex-col border-sidebar-border border-t">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              isActive={pathname === "/app"}
              render={<Link search={{ quickstart: true }} to="/app" />}
              tooltip="Quick start"
            >
              <HugeiconsIcon
                className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
                icon={SparklesIcon}
                size={20}
              />
              <span>Quick start</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton
              render={(props) => (
                <a
                  {...props}
                  href="https://strait.dev/docs"
                  rel="noopener noreferrer"
                  target="_blank"
                >
                  {props.children}
                </a>
              )}
              tooltip="Documentation"
            >
              <HugeiconsIcon
                className="text-muted-foreground/65"
                icon={HelpCircleIcon}
                size={20}
              />
              <span>Documentation</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
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
