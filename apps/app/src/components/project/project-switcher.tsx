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
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { Project } from "@/hooks/api/types";
import {
  projectsQueryOptions,
  useSetActiveProject,
} from "@/hooks/api/use-projects";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { BriefcaseIcon, ChevronDownIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "./create-project-dialog";

type Props = {
  user: AuthUser;
};

const ProjectSwitcher = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const isHydrated = useIsHydrated();
  const setActiveProject = useSetActiveProject();

  const organizationId = user.defaultOrganizationId ?? "";
  const [localActiveProject, setLocalActiveProject] = useState(() => ({
    baseProjectId: user.activeProjectId,
    projectId: user.activeProjectId,
  }));
  const activeProjectId =
    localActiveProject.baseProjectId === user.activeProjectId
      ? localActiveProject.projectId
      : user.activeProjectId;
  const projectPermissions = useProjectPermissions(activeProjectId);
  const { permissions } = projectPermissions;
  const permissionsReady =
    !activeProjectId ||
    projectPermissions.isSuccess ||
    projectPermissions.isError;
  const canCreateProject = !activeProjectId || permissions.canManageProjects;

  const { data: projects } = useQuery({
    ...projectsQueryOptions(organizationId),
    enabled: !!organizationId,
  });

  const activeProject = projects?.find((p) => p.id === activeProjectId);

  const handleSwitch = async (projectId: string) => {
    if (projectId === activeProjectId) {
      setDropdownOpen(false);
      return;
    }
    if (setActiveProject.isPending) {
      return;
    }

    const previousLocalActiveProject = localActiveProject;
    setLocalActiveProject({
      baseProjectId: user.activeProjectId,
      projectId,
    });
    setDropdownOpen(false);
    const switchPromise = setActiveProject.mutateAsync({ projectId });

    toast.promise(switchPromise, {
      loading: "Switching project...",
      success: "Project switched!",
      error: "Failed to switch project",
    });

    try {
      await switchPromise;
    } catch {
      setLocalActiveProject(previousLocalActiveProject);
    }
  };

  const handleCreated = (project: Project) => {
    setLocalActiveProject({
      baseProjectId: user.activeProjectId,
      projectId: project.id,
    });
  };

  if (!organizationId) {
    return null;
  }

  if (!projects || projects.length === 0) {
    return (
      <>
        <SidebarMenuButton
          className="w-full"
          disabled={!isHydrated}
          onClick={() => setCreateOpen(true)}
        >
          <HugeiconsIcon
            className="text-muted-foreground"
            icon={PlusIcon}
            size={18}
          />
          <span className="text-muted-foreground text-sm">
            Create a project
          </span>
        </SidebarMenuButton>
        <CreateProjectDialog
          onCreated={handleCreated}
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
          render={
            <SidebarMenuButton
              className="w-full"
              disabled={!(isHydrated && permissionsReady)}
            />
          }
        >
          <HugeiconsIcon
            className="text-muted-foreground"
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
              onClick={(event) => {
                event.preventDefault();
                handleSwitch(project.id);
              }}
            >
              {project.name}
            </DropdownMenuCheckboxItem>
          ))}
          {canCreateProject && (
            <>
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
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {canCreateProject && (
        <CreateProjectDialog
          onCreated={handleCreated}
          onOpenChange={setCreateOpen}
          open={createOpen}
          organizationId={organizationId}
        />
      )}
    </>
  );
};

export default ProjectSwitcher;
