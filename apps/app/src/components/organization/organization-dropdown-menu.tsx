import {
  Add01Icon,
  Settings01Icon,
  Store01Icon,
  UnfoldMoreIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
} from "@strait/ui/components/avatar";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Sheet, SheetTrigger } from "@strait/ui/components/sheet";
import { SidebarMenuButton } from "@strait/ui/components/sidebar";
import { toast } from "@strait/ui/toast";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import type { OrganizationData } from "@/hooks/auth/use-organization";
import {
  useOrganization,
  useOrganizations,
  useSetDefaultOrganization,
} from "@/hooks/auth/use-organization";
import type { AuthUser, Session } from "@/routes/__root";
import { CreateOrganizationLimitGate } from "./create-organization-limit-gate";
import CreateOrganizationSheet from "./create-organization-sheet";

type Props = {
  user: AuthUser;
  session: Session;
};

const OrganizationDropdownMenu = ({ user, session }: Props) => {
  const navigate = useNavigate();

  const [createOrganizationSheetOpen, setCreateOrganizationSheetOpen] =
    useState<boolean>(false);
  const [dropdownOpen, setDropdownOpen] = useState<boolean>(false);

  // Use suspense query with organizationsQueryOptions for organizations list
  const { data: organizations } = useOrganizations({
    search: {
      perPage: 10,
      page: 1,
      sort: "name_asc",
    },
  });

  // Query for active organization using user data from props
  const { data: activeOrganization } = useOrganization({
    id: session?.user?.defaultOrganizationId ?? "",
  });

  const setActiveOrganization = useSetDefaultOrganization();

  const onSetActiveOrganization = useCallback(
    async (org: OrganizationData) => {
      if (org.id === activeOrganization?.id) {
        return;
      }

      // Prevent multiple concurrent requests
      if (setActiveOrganization.isPending) {
        return;
      }

      // Use toast.promise for better UX
      const switchPromise = setActiveOrganization.mutateAsync({
        id: org.id,
      });

      toast.promise(switchPromise, {
        loading: "Switching active store...",
        success: "Active store changed successfully!",
        error: "Error changing active store",
      });

      try {
        await switchPromise;
        // Close dropdown after successful switch
        setDropdownOpen(false);
      } catch (_error) {
        // Error toast is already handled by toast.promise
      }
    },
    [activeOrganization, setActiveOrganization]
  );

  // Handle case where user has no organizations (needs onboarding)
  if (
    !activeOrganization &&
    (!organizations?.page || organizations.page.length === 0)
  ) {
    return (
      <Sheet
        onOpenChange={setCreateOrganizationSheetOpen}
        open={createOrganizationSheetOpen}
      >
        <SheetTrigger
          render={
            <SidebarMenuButton className="w-full max-w-[200px]" size="lg" />
          }
        >
          <Avatar className="h-10 w-10">
            <AvatarFallback>
              <HugeiconsIcon className="size-4" icon={Add01Icon} />
            </AvatarFallback>
          </Avatar>
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-semibold">
              Create first organization
            </span>
            <span className="truncate text-muted-foreground text-xs">
              You don't have any organizations yet
            </span>
          </div>
          <HugeiconsIcon className="ml-auto size-4" icon={UnfoldMoreIcon} />
        </SheetTrigger>
        <CreateOrganizationSheet
          onClose={() => setCreateOrganizationSheetOpen(false)}
          user={user}
        />
      </Sheet>
    );
  }

  // Early return if activeOrganization is still loading
  // This should be rare since organizations are suspended above
  if (!activeOrganization) {
    return null;
  }

  return (
    <Sheet
      onOpenChange={() =>
        setCreateOrganizationSheetOpen(!createOrganizationSheetOpen)
      }
      open={createOrganizationSheetOpen}
    >
      <DropdownMenu onOpenChange={setDropdownOpen} open={dropdownOpen}>
        <DropdownMenuTrigger
          render={
            <SidebarMenuButton className="w-full max-w-[200px]" size="lg" />
          }
        >
          <Avatar className="h-10 w-10">
            {activeOrganization.logo ? (
              <AvatarImage src={activeOrganization.logo} />
            ) : null}
            <AvatarFallback>
              <HugeiconsIcon className="size-4" icon={Store01Icon} />
            </AvatarFallback>
          </Avatar>

          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-semibold">
              {activeOrganization.name}
            </span>
            <span className="truncate text-muted-foreground text-xs">
              {activeOrganization.slug}
            </span>
          </div>

          <HugeiconsIcon className="size-4" icon={UnfoldMoreIcon} />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="w-[--radix-dropdown-menu-trigger-width] min-w-56 rounded-custom"
          side="bottom"
          sideOffset={4}
        >
          <DropdownMenuGroup>
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                <Avatar className="h-10 w-10">
                  {activeOrganization.logo ? (
                    <AvatarImage src={activeOrganization.logo} />
                  ) : null}
                  <AvatarFallback>
                    <HugeiconsIcon className="size-4" icon={Store01Icon} />
                  </AvatarFallback>
                </Avatar>

                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">
                    {activeOrganization.name}
                  </span>
                  <span className="truncate text-muted-foreground text-xs">
                    {activeOrganization.slug}
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
          </DropdownMenuGroup>

          <DropdownMenuSeparator />
          {organizations?.page?.map((organization: OrganizationData) => (
            <DropdownMenuCheckboxItem
              checked={organization.id === activeOrganization?.id}
              disabled={setActiveOrganization.isPending}
              key={organization.id}
              onSelect={(e) => {
                e.preventDefault();
                onSetActiveOrganization(organization);
              }}
            >
              {organization.name}
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onSelect={(e) => {
              e.preventDefault();
              navigate({ to: "/app/settings" });
            }}
          >
            <HugeiconsIcon className="size-4" icon={Settings01Icon} />
            Settings
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <CreateOrganizationLimitGate
            currentCount={organizations?.page?.length || 0}
            onUpgradeClick={() => {
              setDropdownOpen(false);
              navigate({ to: "/app/upgrade" });
            }}
          >
            <SheetTrigger
              render={
                <DropdownMenuItem
                  onSelect={(e) => {
                    e.preventDefault();
                    setCreateOrganizationSheetOpen(true);
                  }}
                />
              }
            >
              <HugeiconsIcon className="size-4" icon={Add01Icon} />
              Create new store
            </SheetTrigger>
          </CreateOrganizationLimitGate>
        </DropdownMenuContent>
      </DropdownMenu>

      <CreateOrganizationSheet
        onClose={() => setCreateOrganizationSheetOpen(false)}
        user={user}
      />
    </Sheet>
  );
};

export default OrganizationDropdownMenu;
