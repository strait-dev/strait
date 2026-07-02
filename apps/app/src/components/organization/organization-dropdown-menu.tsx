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
import { SidebarMenuButton } from "@strait/ui/components/sidebar";
import { toast } from "@strait/ui/components/toast";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { useState } from "react";
import {
  projectsQueryOptions,
  useSetActiveProject,
} from "@/hooks/api/use-projects";
import type { OrganizationData } from "@/hooks/auth/use-organization";
import {
  useOrganization,
  useOrganizations,
  useSetDefaultOrganization,
} from "@/hooks/auth/use-organization";
import {
  BuildingIcon,
  PlusIcon,
  SettingsOutlineIcon,
  UnfoldMoreIcon,
} from "@/lib/icons";
import type { AuthUser, Session } from "@/routes/__root";
import CreateOrganizationDialog from "./create-organization-dialog";

type Props = {
  user: AuthUser;
  session: Session;
};

const OrganizationDropdownMenu = ({ user, session }: Props) => {
  const navigate = useNavigate();

  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);

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
  const setActiveProject = useSetActiveProject();
  const queryClient = useQueryClient();
  const router = useRouter();

  const onSetActiveOrganization = async (org: OrganizationData) => {
    if (org.id === activeOrganization?.id) {
      return;
    }

    if (setActiveOrganization.isPending) {
      return;
    }

    const switchPromise = (async () => {
      await setActiveOrganization.mutateAsync({ id: org.id });

      // Auto-select the first project in the new org
      const projects = await queryClient.fetchQuery(
        projectsQueryOptions(org.id)
      );
      if (projects && projects.length > 0) {
        await setActiveProject.mutateAsync({ projectId: projects[0].id });
      }

      await queryClient.invalidateQueries();
      router.invalidate();
    })();

    toast.promise(switchPromise, {
      loading: "Switching organization...",
      success: "Organization switched successfully!",
      error: "Error switching organization",
    });

    try {
      await switchPromise;
      setDropdownOpen(false);
    } catch {
      // Error toast is already handled by toast.promise
    }
  };

  // Handle case where user has no organizations (needs onboarding)
  if (
    !activeOrganization &&
    (!organizations?.page || organizations.page.length === 0)
  ) {
    return (
      <>
        <SidebarMenuButton
          className="w-full"
          onClick={() => setCreateDialogOpen(true)}
          size="lg"
        >
          <Avatar className="size-10">
            <AvatarFallback>
              <HugeiconsIcon className="size-4" icon={PlusIcon} />
            </AvatarFallback>
          </Avatar>
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-normal">
              Create first organization
            </span>
            <span className="truncate text-muted-foreground text-xs">
              You don't have any organizations yet
            </span>
          </div>
          <HugeiconsIcon className="ml-auto size-4" icon={UnfoldMoreIcon} />
        </SidebarMenuButton>
        <CreateOrganizationDialog
          onClose={() => setCreateDialogOpen(false)}
          onOpenChange={setCreateDialogOpen}
          open={createDialogOpen}
          user={user}
        />
      </>
    );
  }

  // Early return if activeOrganization is still loading
  // This should be rare since organizations are suspended above
  if (!activeOrganization) {
    return null;
  }

  return (
    <>
      <DropdownMenu onOpenChange={setDropdownOpen} open={dropdownOpen}>
        <DropdownMenuTrigger
          render={<SidebarMenuButton className="w-full" size="lg" />}
        >
          <Avatar className="size-10">
            {activeOrganization.logo ? (
              <AvatarImage src={activeOrganization.logo} />
            ) : null}
            <AvatarFallback>
              <HugeiconsIcon className="size-4" icon={BuildingIcon} />
            </AvatarFallback>
          </Avatar>

          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-normal">
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
          className="w-[--radix-dropdown-menu-trigger-width] min-w-56"
          side="bottom"
          sideOffset={4}
        >
          <DropdownMenuGroup>
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                <Avatar className="size-10">
                  {activeOrganization.logo ? (
                    <AvatarImage src={activeOrganization.logo} />
                  ) : null}
                  <AvatarFallback>
                    <HugeiconsIcon className="size-4" icon={BuildingIcon} />
                  </AvatarFallback>
                </Avatar>

                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-normal">
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
            onClick={() => {
              setDropdownOpen(false);
              navigate({ to: `/app/org/${activeOrganization.id}` });
            }}
          >
            <HugeiconsIcon className="size-4" icon={SettingsOutlineIcon} />
            Settings
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onSelect={(e) => {
              e.preventDefault();
              setDropdownOpen(false);
              setCreateDialogOpen(true);
            }}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create new organization
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <CreateOrganizationDialog
        onClose={() => setCreateDialogOpen(false)}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
        user={user}
      />
    </>
  );
};

export default OrganizationDropdownMenu;
