import { HugeiconsIcon } from "@hugeicons/react";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { SidebarMenuButton } from "@strait/ui/components/sidebar";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { projectsQueryOptions, useSetActiveProject } from "@/hooks/api/use-projects";
import {
  BriefcaseIcon,
  ChevronDownIcon,
  PlusIcon,
} from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "./create-project-dialog";

type Props = {
  user: AuthUser;
};

const ProjectSwitcher = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const setActiveProject = useSetActiveProject();
  const queryClient = useQueryClient();
  const router = useRouter();

  const organizationId = user.defaultOrganizationId ?? "";
  const activeProjectId = user.activeProjectId;

  const { data: projects } = useQuery({
    ...projectsQueryOptions(organizationId),
    enabled: !!organizationId,
  });

  const activeProject = projects?.find((p) => p.id === activeProjectId);

  const handleSwitch = useCallback(
    async (projectId: string) => {
      if (projectId === activeProjectId) return;
      if (setActiveProject.isPending) return;

      const switchPromise = setActiveProject.mutateAsync({ projectId });

      toast.promise(switchPromise, {
        loading: "Switching project...",
        success: "Project switched!",
        error: "Failed to switch project",
      });

      try {
        await switchPromise;
        await queryClient.invalidateQueries();
        router.invalidate();
        setDropdownOpen(false);
      } catch {
        // handled by toast
      }
    },
    [activeProjectId, setActiveProject, queryClient, router]
  );

  if (!organizationId) {
    return null;
  }

  if (!projects || projects.length === 0) {
    return (
      <>
        <SidebarMenuButton
          className="w-full"
          onClick={() => setCreateOpen(true)}
        >
          <HugeiconsIcon
            className="text-muted-foreground/65"
            icon={PlusIcon}
            size={18}
          />
          <span className="text-muted-foreground text-sm">Create a project</span>
        </SidebarMenuButton>
        <CreateProjectDialog
          onOpenChange={setCreateOpen}
          open={createOpen}
          organizationId={organizationId}
        />
      </>
    );
  }

  return (
    <>
      <DropdownMenu onOpenChange={setDropdownOpen} open={dropdownOpen}>
        <DropdownMenuTrigger
          render={<SidebarMenuButton className="w-full" />}
        >
          <HugeiconsIcon
            className="text-muted-foreground/65"
            icon={BriefcaseIcon}
            size={18}
          />
          <span className="flex-1 truncate text-sm">
            {activeProject?.name ?? "Select project"}
          </span>
          <HugeiconsIcon
            className="size-3 text-muted-foreground"
            icon={ChevronDownIcon}
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-48" sideOffset={4}>
          {projects.map((project) => (
            <DropdownMenuCheckboxItem
              checked={project.id === activeProjectId}
              disabled={setActiveProject.isPending}
              key={project.id}
              onSelect={(e) => {
                e.preventDefault();
                handleSwitch(project.id);
              }}
            >
              {project.name}
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => {
              setDropdownOpen(false);
              setCreateOpen(true);
            }}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            New project
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <CreateProjectDialog
        onOpenChange={setCreateOpen}
        open={createOpen}
        organizationId={organizationId}
      />
    </>
  );
};

export default ProjectSwitcher;
