import {
  ArrowRight01Icon,
  BankIcon,
  BarCode01Icon,
  FolderLibraryIcon,
  HelpCircleIcon,
  Home07Icon,
  PieChartIcon,
  SentIcon,
  ShoppingBag01Icon,
  ShoppingCart02Icon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@strait/ui/components/collapsible";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
} from "@strait/ui/components/sidebar";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useCallback, useMemo } from "react";
import { FEATURE_FLAGS } from "@/hooks/posthog/flags";
import { useFeatureFlag } from "@/hooks/posthog/use-feature-flag";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import type { Session } from "@/routes/__root";
import PaymentPendingCard from "../subscription/payment-pending-card";
import TrialUpgradeCard from "../subscription/trial-upgrade-card";
import UserDropdownMenu from "./user-dropdown-menu";

/**
 * Feature keys used for sidebar navigation gating
 */
type SidebarFeature =
  | "stockControl"
  | "stockTransfers"
  | "returns"
  | "stockCounts";

const data = {
  nav: [
    { title: "Home", url: "/app", icon: Home07Icon, isActive: true },
    {
      title: "Reports",
      url: "/app/reports",
      isActive: false,
      icon: PieChartIcon,
      items: [
        { title: "Sales", url: "/app/reports/sales" },
        { title: "Customers", url: "/app/reports/customers" },
        { title: "Finance", url: "/app/reports/finance" },
        { title: "Suppliers", url: "/app/reports/suppliers" },
        { title: "Inventory", url: "/app/reports/inventory" },
        { title: "Catalog", url: "/app/reports/catalog" },
      ],
    },
    {
      title: "Checkout",
      url: "/app/checkout",
      isActive: false,
      icon: BarCode01Icon,
    },
    {
      title: "Sales",
      url: "/app/orders",
      isActive: false,
      icon: ShoppingCart02Icon,
      items: [
        { title: "General", url: "/app/orders" },
        { title: "Registers", url: "/app/registers" },
      ],
    },
    {
      title: "Inventory",
      url: "/app/inventory",
      isActive: false,
      icon: ShoppingBag01Icon,
      requiredFeature: "stockControl" as SidebarFeature,
      items: [
        { title: "Products", url: "/app/products" },
        {
          title: "Purchases",
          url: "/app/purchases",
          requiredFeature: "stockControl" as SidebarFeature,
        },
        {
          title: "Transfers",
          url: "/app/transfers",
          requiredFeature: "stockTransfers" as SidebarFeature,
        },
        {
          title: "Returns",
          url: "/app/returns",
          requiredFeature: "returns" as SidebarFeature,
        },
        {
          title: "Counts",
          url: "/app/counts",
          requiredFeature: "stockCounts" as SidebarFeature,
        },
      ],
    },
    {
      title: "Catalog",
      url: "/app/products",
      isActive: false,
      icon: FolderLibraryIcon,
      items: [
        { title: "Services", url: "/app/services" },
        { title: "Brands", url: "/app/brands" },
        { title: "Promotions", url: "/app/promotions" },
        { title: "Tags", url: "/app/tags" },
        { title: "Groups", url: "/app/groups" },
      ],
    },
    {
      title: "People",
      url: "/app/customers",
      isActive: false,
      icon: UserGroupIcon,
      items: [
        { title: "Customers", url: "/app/customers" },
        { title: "Suppliers", url: "/app/suppliers" },
      ],
    },
    {
      title: "Financial",
      url: "/app/finance",
      isActive: false,
      icon: BankIcon,
      items: [
        { title: "Payments", url: "/app/payments" },
        { title: "Receipts", url: "/app/receipts" },
      ],
    },
  ],
  navSecondary: [
    {
      title: "Support",
      url: "#",
      isActive: false,
      icon: HelpCircleIcon,
    },
    {
      title: "Feedback",
      url: "#",
      isActive: false,
      icon: SentIcon,
    },
  ],
};

type Props = {
  session: NonNullable<Session>;
};

type NavigationItem = {
  title: string;
  url: string;
  icon?: any;
  isActive: boolean;
  requiredFeature?: SidebarFeature;
  requiredValue?: string | string[];
  items?: {
    title: string;
    url: string;
    isActive?: boolean;
    requiredFeature?: SidebarFeature;
    requiredValue?: string | string[];
  }[];
};

type SubItem = NavigationItem["items"] extends (infer T)[] | undefined
  ? T
  : never;

// Sub-component for navigation icon
const NavIcon = ({
  icon,
  isDisabled,
}: {
  icon: NavigationItem["icon"];
  isDisabled?: boolean;
}) =>
  icon ? (
    <HugeiconsIcon
      aria-hidden="true"
      className={
        isDisabled
          ? "text-muted-foreground/65"
          : "text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
      }
      icon={icon}
      size={22}
    />
  ) : null;

// Sub-component for Pro badge
const ProBadge = () => (
  <Badge className="ml-auto" variant="info-light">
    Pro
  </Badge>
);

// Sub-component for submenu items
const SubMenuItems = ({
  items,
  canAccessNavItem,
  shouldShowProBadge,
}: {
  items: SubItem[];
  canAccessNavItem: (item: NavigationItem) => boolean;
  shouldShowProBadge: (item: NavigationItem) => boolean;
}) => (
  <SidebarMenuSub>
    {items.map((subItem) => {
      const itemWithActive = { ...subItem, isActive: false };
      const hasAccess = canAccessNavItem(itemWithActive);

      return (
        <SidebarMenuSubItem key={subItem.title}>
          {hasAccess ? (
            <SidebarMenuSubButton render={<Link to={subItem.url} />}>
              <span>{subItem.title}</span>
            </SidebarMenuSubButton>
          ) : (
            <SidebarMenuSubButton className="pointer-events-none cursor-default opacity-50">
              <span>{subItem.title}</span>
              {shouldShowProBadge(itemWithActive) ? <ProBadge /> : null}
            </SidebarMenuSubButton>
          )}
        </SidebarMenuSubItem>
      );
    })}
  </SidebarMenuSub>
);

// Sub-component for collapsible nav item with sub-items
const CollapsibleNavItem = ({
  item,
  canAccessNavItem,
  shouldShowProBadge,
}: {
  item: NavigationItem;
  canAccessNavItem: (item: NavigationItem) => boolean;
  shouldShowProBadge: (item: NavigationItem) => boolean;
}) => (
  <Collapsible
    className="group/collapsible"
    defaultOpen={item.isActive}
    render={<SidebarMenuItem />}
  >
    <CollapsibleTrigger
      render={
        <SidebarMenuButton
          disabled={!canAccessNavItem(item)}
          tooltip={item.title}
        />
      }
    >
      <NavIcon icon={item.icon} />
      <span>{item.title}</span>
      <HugeiconsIcon
        className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
        icon={ArrowRight01Icon}
      />
    </CollapsibleTrigger>
    <CollapsibleContent>
      <SubMenuItems
        canAccessNavItem={canAccessNavItem as any}
        items={item.items || []}
        shouldShowProBadge={shouldShowProBadge as any}
      />
    </CollapsibleContent>
  </Collapsible>
);

// Sub-component for simple nav item without sub-items
const SimpleNavItem = ({
  item,
  canAccessNavItem,
  shouldShowProBadge,
}: {
  item: NavigationItem;
  canAccessNavItem: (item: NavigationItem) => boolean;
  shouldShowProBadge: (item: NavigationItem) => boolean;
}) => {
  const hasAccess = canAccessNavItem(item);

  return (
    <SidebarMenuItem>
      {hasAccess ? (
        <SidebarMenuButton
          disabled={!hasAccess}
          render={<Link to={item.url} />}
          tooltip={item.title}
        >
          <NavIcon icon={item.icon} />
          <span>{item.title}</span>
        </SidebarMenuButton>
      ) : (
        <SidebarMenuButton
          className="pointer-events-none cursor-default opacity-50"
          tooltip={item.title}
        >
          <NavIcon icon={item.icon} isDisabled />
          <span>{item.title}</span>
          {shouldShowProBadge(item) ? <ProBadge /> : null}
        </SidebarMenuButton>
      )}
    </SidebarMenuItem>
  );
};

const AppSidebar = ({ session }: Props) => {
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { shouldShowUpgrade, hasPendingPayment } = subscriptionState;

  // Use PostHog feature flags for access control
  const hasStockCounts = useFeatureFlag(FEATURE_FLAGS.STOCK_COUNTS);
  const hasStockTransfers = useFeatureFlag(FEATURE_FLAGS.STOCK_TRANSFERS);
  const hasReturns = useFeatureFlag(FEATURE_FLAGS.RETURNS);

  // Feature access map using PostHog hooks
  // Note: "stockControl" and "stockCounts" both use the same STOCK_COUNTS flag
  const featureAccessMap = useMemo(() => {
    const map = new Map<string, { hasAccess: boolean }>();

    // Map features to their access status using PostHog
    map.set("stockControl", {
      hasAccess: hasStockCounts,
    });
    map.set("stockTransfers", {
      hasAccess: hasStockTransfers,
    });
    map.set("returns", {
      hasAccess: hasReturns,
    });
    map.set("stockCounts", {
      hasAccess: hasStockCounts,
    });

    return map;
  }, [hasStockCounts, hasStockTransfers, hasReturns]);

  // Function to check if a navigation item should be accessible (without development override)
  const hasFeatureAccess = useCallback(
    (item: NavigationItem) => {
      if (!item.requiredFeature) {
        return true;
      }

      const featureAccess = featureAccessMap.get(item.requiredFeature);
      if (!featureAccess?.hasAccess) {
        return false;
      }

      return true;
    },
    [featureAccessMap]
  );

  // Function to check if a navigation item should be accessible (with development override)
  const canAccessNavItem = useCallback(
    (item: NavigationItem) => {
      if (process.env.NODE_ENV === "development") {
        return true;
      }

      return hasFeatureAccess(item);
    },
    [hasFeatureAccess]
  );

  // Function to check if Pro badge should be shown with upgrade plan info
  const shouldShowProBadge = useCallback(
    (item: NavigationItem) => {
      if (process.env.NODE_ENV === "development") {
        return false;
      }

      if (!item.requiredFeature) {
        return false;
      }

      return !hasFeatureAccess(item);
    },
    [hasFeatureAccess]
  );

  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader className="h-16 border-sidebar-border border-b">
        <div className="flex h-full w-full items-center px-2">
          <Link search={{ subscription: undefined, t: undefined }} to="/app">
            <span className="sr-only">Strait</span>
            <img
              alt="Strait logo"
              className="h-8 w-auto"
              height={20}
              src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9FHxThgYVE9uncAThKs1v37lk5QJHeDdzbPmr"
              width={20}
            />
          </Link>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Store</SidebarGroupLabel>
          <SidebarMenu>
            {data.nav
              .filter(canAccessNavItem)
              .map((item) =>
                !!item.items && item.items.length > 0 ? (
                  <CollapsibleNavItem
                    canAccessNavItem={canAccessNavItem}
                    item={item}
                    key={item.title}
                    shouldShowProBadge={shouldShowProBadge}
                  />
                ) : (
                  <SimpleNavItem
                    canAccessNavItem={canAccessNavItem}
                    item={item}
                    key={item.title}
                    shouldShowProBadge={shouldShowProBadge}
                  />
                )
              )}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      {hasPendingPayment ? <PaymentPendingCard /> : null}
      {shouldShowUpgrade ? <TrialUpgradeCard /> : null}

      <SidebarFooter className="flex flex-col border-sidebar-border border-t">
        <UserDropdownMenu user={session.user} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
};

export default AppSidebar;
