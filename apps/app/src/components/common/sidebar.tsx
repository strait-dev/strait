import { HugeiconsIcon } from "@hugeicons/react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@strait/ui/components/collapsible";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@strait/ui/components/command";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";
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
  SidebarSeparator,
} from "@strait/ui/components/sidebar";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import { Suspense, useEffect, useState } from "react";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { isCommunityEdition } from "@/lib/edition";
import {
  ChevronDownIcon,
  CreditCardIcon,
  HelpCircleIcon,
  SearchIcon,
} from "@/lib/icons";
import type { Session } from "@/routes/__root";
import OrganizationDropdownMenu from "../organization/organization-dropdown-menu";
import ProjectSwitcher from "../project/project-switcher";
import PaymentPendingCard from "../subscription/payment-pending-card";
import TemporaryAccessUpgradeCard from "../subscription/temporary-access-upgrade-card";
import {
  buildQuickCreateCommands,
  buildSidebarCommandGroups,
  commandValue,
  mainNav,
  type NavItem,
  observabilityNav,
  type SidebarCommandItem,
} from "./sidebar-commands";

type Props = {
  session: NonNullable<Session>;
};

const AppSidebar = ({ session }: Props) => {
  const navigate = useNavigate();
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { shouldShowUpgrade, hasPendingPayment } = subscriptionState;
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const [commandOpen, setCommandOpen] = useState(false);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() === "k" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setCommandOpen(true);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const pathname = useRouterState({
    select: (s) => s.location.pathname,
  });
  const orgSettingsRoute = session.user.defaultOrganizationId
    ? `/app/org/${session.user.defaultOrganizationId}`
    : "/app/settings";

  const quickActions = buildQuickCreateCommands(permissions);

  const commandGroups = buildSidebarCommandGroups(
    permissions,
    orgSettingsRoute
  );

  const runCommand = (item: SidebarCommandItem) => {
    if (item.href) {
      globalThis.location.assign(item.href);
      return;
    }
    if (item.url) {
      navigate({ to: item.url });
    }
  };

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
      <SidebarHeader className="h-16">
        <div className="flex h-full w-full items-center px-2">
          <Link to="/app">
            <span className="sr-only">Strait</span>
            <img
              alt="Strait logo"
              className="h-8 w-auto"
              height={20}
              src="/strait-logo-black.svg"
              width={20}
            />
          </Link>
        </div>
      </SidebarHeader>
      <SidebarSeparator />

      <SidebarContent className="pt-2">
        <SidebarGroup>
          <SidebarGroupLabel>Project</SidebarGroupLabel>
          <SidebarMenu>
            <SidebarMenuItem>
              <ProjectSwitcher user={session.user} />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup>
          <button
            className="group/search flex h-8 w-full items-center gap-2 rounded-md border border-sidebar-border bg-transparent px-2 text-left text-muted-foreground text-sm outline-none ring-sidebar-ring transition-colors hover:bg-sidebar-accent/40 hover:text-sidebar-accent-foreground focus-visible:ring-3"
            onClick={() => setCommandOpen(true)}
            type="button"
          >
            <HugeiconsIcon className="size-4 shrink-0" icon={SearchIcon} />
            <span className="flex-1 truncate">Search...</span>
            <kbd className="rounded border border-sidebar-border px-1.5 font-mono text-micro">
              ⌘K
            </kbd>
          </button>
          <div className="mt-2 space-y-1">
            <input
              aria-label="Command palette"
              className="h-8 w-full rounded-md border border-sidebar-border bg-transparent px-2 text-sm outline-none focus-visible:ring-3 focus-visible:ring-sidebar-ring"
              placeholder="Type a command..."
            />
            {quickActions.length > 0 ? (
              <div className="grid gap-1">
                {quickActions.map((item) => {
                  const Icon = item.icon;
                  return (
                    <SidebarMenuButton
                      key={item.label}
                      render={(props) => (
                        <a {...props} href={item.href}>
                          {props.children}
                        </a>
                      )}
                      size="sm"
                    >
                      {Icon ? (
                        <HugeiconsIcon className="size-4" icon={Icon} />
                      ) : null}
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  );
                })}
              </div>
            ) : null}
          </div>
          <Dialog onOpenChange={setCommandOpen} open={commandOpen}>
            <DialogContent
              className="top-1/3 translate-y-0 overflow-hidden rounded-lg! p-0"
              showCloseButton={false}
            >
              <DialogHeader className="sr-only">
                <DialogTitle>Command palette</DialogTitle>
                <DialogDescription>
                  Search for a command to run...
                </DialogDescription>
              </DialogHeader>
              <Command>
                <CommandInput
                  aria-label="Command search"
                  cmdk-input=""
                  placeholder="Search..."
                />
                <CommandList>
                  <CommandEmpty>No results found.</CommandEmpty>
                  {commandGroups.map((group) => (
                    <CommandGroup heading={group.heading} key={group.heading}>
                      {group.items.map((item) => (
                        <CommandItem
                          key={item.label}
                          onSelect={() => {
                            runCommand(item);
                            setCommandOpen(false);
                          }}
                          value={commandValue(item)}
                        >
                          {item.icon ? (
                            <HugeiconsIcon icon={item.icon} size={16} />
                          ) : null}
                          <span>{item.label}</span>
                          {item.shortcut ? (
                            <CommandShortcut>{item.shortcut}</CommandShortcut>
                          ) : null}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  ))}
                </CommandList>
              </Command>
            </DialogContent>
          </Dialog>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarMenu>
            {mainNav.map((item) => (
              <SidebarMenuItem key={item.url}>
                <SidebarMenuButton
                  active={isActive(item)}
                  render={<Link reloadDocument to={item.url} />}
                  tooltip={item.title}
                >
                  <HugeiconsIcon
                    className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
                    icon={item.icon}
                    size={22}
                  />
                  <span>{item.title}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroup>

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
                        active={isActive(item)}
                        render={<Link reloadDocument to={item.url} />}
                        tooltip={item.title}
                      >
                        <HugeiconsIcon
                          className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
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

        {isCommunityEdition ? null : (
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  active={
                    pathname === "/app/billing" ||
                    pathname.startsWith("/app/billing/")
                  }
                  render={<Link reloadDocument to="/app/billing" />}
                  tooltip="Billing"
                >
                  <HugeiconsIcon
                    className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
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
      {!isCommunityEdition && shouldShowUpgrade ? (
        <TemporaryAccessUpgradeCard />
      ) : null}

      <SidebarSeparator />
      <SidebarFooter className="flex flex-col">
        <SidebarMenu>
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
                className="text-muted-foreground"
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
